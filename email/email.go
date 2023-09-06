package email

import (
	"blog/limiter"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var cfkey = os.Getenv("CFKEY") // cloudflare key
var sepRemover = strings.NewReplacer("-", "", " ", "", ".", "")

var msgauth struct {
	active *limiter.ActiveLimiter

	sync.Mutex
	// waiters is a map of shortid to email address channel.
	waiters map[int]chan<- string
}

func init() {
	msgauth.waiters = make(map[int]chan<- string)
	msgauth.active = limiter.NewActiveLimiter(50)
}

func respond(w http.ResponseWriter, code int, format string, args ...any) {
	log.Printf("msgauth response: %s: %s", http.StatusText(code), fmt.Sprintf(format, args...))
	w.WriteHeader(code)
	fmt.Fprintf(w, format, args...)
}

func HandleMsgauthwait(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/plain")
	if err := req.ParseForm(); err != nil {
		respond(w, http.StatusBadRequest, "parse form: %v", err)
		return
	}
	id, err := strconv.Atoi(sepRemover.Replace(req.Form.Get("id")))
	if err != nil {
		respond(w, http.StatusBadRequest, "parse id: %v", err)
		return
	}

	// handle logins.
	// the email notification comes from the cloudflare worker.
	if req.Form.Has("login") {
		if req.Header.Get("cfkey") != cfkey {
			respond(w, http.StatusBadRequest, "invalid cfkey")
			return
		}
		msgauth.Lock()
		ch, ok := msgauth.waiters[id]
		msgauth.Unlock()
		if !ok {
			respond(w, http.StatusGone, "no waiter for code %d", id)
			return
		}
		ch <- req.Form.Get("from")
		respond(w, http.StatusOK, "ok")
		return
	}

	if !msgauth.active.Add() {
		respond(w, http.StatusServiceUnavailable, "msgauth service overloaded")
		return
	}
	defer msgauth.active.Finish()
	ch := make(chan string)
	msgauth.Lock()
	msgauth.waiters[id] = ch
	msgauth.Unlock()
	defer func() {
		msgauth.Lock()
		delete(msgauth.waiters, id)
		msgauth.Unlock()
	}()
	select {
	case email := <-ch:
		http.ServeContent(w, req, "msgauthwait", time.Time{}, strings.NewReader(email))
		log.Printf("responded to /msgauthwait?id=%d with %q.", id, email)
	case <-req.Context().Done():
	}
}
