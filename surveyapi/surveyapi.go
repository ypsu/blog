package surveyapi

import (
	"blog/abname"
	"blog/alogdb"
	"blog/eventz"
	"blog/userapi"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SurveyHandler struct {
	// Maps surveyname to its metadata.
	surveyStatus map[string]status

	// Protects responses, lastResponseEpoch, and recentUIDs.
	mu sync.Mutex

	// Maps responseID (surveyname.username) to the response json.
	responses map[string]string

	// The epoch and the submissions in the epoch for ratelimiting purposes.
	lastResponseEpoch int64
	recentUIDs        []abname.ID

	adb *alogdb.DB
	udb *userapi.DB
	ez  *eventz.EventZ
}

type status struct {
	end time.Time
}

func New(adb *alogdb.DB, udb *userapi.DB, ez *eventz.EventZ) *SurveyHandler {
	sh := &SurveyHandler{
		surveyStatus: map[string]status{
			"survey26": {
				end: time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		responses: map[string]string{},
		adb:       adb,
		udb:       udb,
		ez:        ez,
	}

	for _, e := range adb.Get("surveys") {
		responseID, response, ok := strings.Cut(e.Text, " ")
		if !ok {
			log.Printf("surveyapi.BadSurveyData ts=%d: %s", e.TS, e.Text)
			continue
		}
		sh.responses[responseID] = response
	}
	return sh
}

func (sh *SurveyHandler) HandleHTTP(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	u, s := sh.udb.Username(w, req), q.Get("survey")
	if u == "" {
		http.Error(w, "surveyapi.NotLoggedIn", http.StatusUnauthorized)
		return
	}
	if s == "" {
		http.Error(w, "surveyapi.MissingSurveyID", http.StatusBadRequest)
		return
	}
	ss, found := sh.surveyStatus[s]
	if !found {
		http.Error(w, "surveyapi.SurveyNotFound survey="+s, http.StatusNotFound)
		return
	}
	if req.ContentLength == -1 {
		sh.ez.Printf("surveyapi.MissingContentLength user=%s survey=%s", u, s)
		http.Error(w, "surveyapi.MissingContentLength", http.StatusLengthRequired)
		return
	}
	if req.ContentLength > 10_000 {
		sh.ez.Printf("surveyapi.SurveyResponseTooLong user=%s survey=%s gotlen=%d", u, s, req.ContentLength)
		http.Error(w, fmt.Sprintf("surveyapi.SurveyResponseTooLong gotlen=%d limit=1e4: make some answers shorter", req.ContentLength), http.StatusRequestEntityTooLarge)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		sh.ez.Printf("surveyapi.FailedSurveyRead user=%s survey=%s err=%s content:\n%s", u, s, err, body)
		http.Error(w, "surveyapi.FailedSurveyRead: "+err.Error(), http.StatusInternalServerError)
		return
	}
	buf := &bytes.Buffer{}
	if err := json.Compact(buf, body); len(body) > 0 && err != nil {
		sh.ez.Printf("surveyapi.NonJSONBody user=%s survey=%s err=%s content: %s", u, s, err, body)
		http.Error(w, "surveyapi.NonJSONBody: "+err.Error(), http.StatusBadRequest)
		return
	}
	js := buf.String()
	uid, err := abname.New(u)
	if err != nil {
		http.Error(w, fmt.Sprintf("surveyapi.AbnameForUser user=%s: %v", u, err), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if req.Method == "GET" {
		if u == "iio" && q.Has("all") {
			s += "."
			w.Header().Set("Content-Type", "text/plain")
			for id, rsp := range sh.responses {
				if !strings.HasPrefix(id, s) {
					continue
				}
				fmt.Fprintf(w, "%s %s\n", id, rsp)
			}
			return
		}

		rsp, found := sh.responses[s+"."+u]
		if !found {
			http.Error(w, "surveyapi.SurveyResponseNotFound", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, rsp)
		return
	}

	if req.Method != "POST" {
		http.Error(w, "surveyapi.BadSurveyRequest", http.StatusBadRequest)
		return
	}

	if now.After(ss.end) {
		http.Error(w, fmt.Sprintf("surveyapi.SurveyClosed survey=%s: this survey is now closed", s), http.StatusLocked)
		return
	}

	// Ratelimit.
	epoch := now.Unix() / 512
	if epoch != sh.lastResponseEpoch {
		sh.lastResponseEpoch, sh.recentUIDs = epoch, sh.recentUIDs[:0]
	}
	uidCount := 0
	for _, id := range sh.recentUIDs {
		if id == uid {
			uidCount++
		}
	}
	if uidCount == 3 {
		sh.ez.Printf("surveyapi.TooFrequentUpdates user=%s survey=%s: %s", u, s, body)
		http.Error(w, "surveyapi.TooFrequentUpdates", http.StatusTooManyRequests)
		return
	}
	if c := len(sh.recentUIDs); uidCount == 0 && (c > 20 || c > 10 && rand.Intn(4) == 0) {
		sh.ez.Printf("surveyapi.TooManySurveyResponses user=%s survey=%s c=%d: %s", u, s, c, body)
		http.Error(w, "surveyapi.TooManySurveyResponses: this is surprisingly popular or someone is spamming me, but try again 10 minutes later", http.StatusTooManyRequests)
		return
	}
	sh.recentUIDs = append(sh.recentUIDs, uid)

	if _, err := sh.adb.Add("surveys", s+"."+u+" "+js); err != nil {
		sh.ez.Printf("surveyapi.SaveSurveyResponse: user=%s survey=%s body=%s; error: %v", u, s, js, err)
		http.Error(w, "surveyapi.SaveSurveyFailed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sh.responses[s+"."+u] = js
	sh.ez.Printf("surveyapi.Response user=%s survey=%s: %s", u, s, js)
}
