package email

import (
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
)

var sepRemover = strings.NewReplacer("-", "", " ", "", ".", "")

var msgauth struct {
	sync.Mutex
	// waiters is a map of shortid to email address channel.
	waiters map[int]chan<- string
}

func init() {
	msgauth.waiters = make(map[int]chan<- string)
}

func HandleMsgauthwait(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/plain")
	if err := req.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400 bad request: parse form: %v\n", err)
		return
	}
	id, err := strconv.Atoi(sepRemover.Replace(req.Form.Get("id")))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400 bad request: parse id: %v\n", err)
		return
	}
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

func handleMsgauthEmail(from string, msg *mail.Message) error {
	id, err := strconv.Atoi(sepRemover.Replace(msg.Header.Get("Subject")))
	if err != nil {
		return fmt.Errorf("parse subject %q: %w", msg.Header.Get("Subject"), err)
	}
	msgauth.Lock()
	ch, ok := msgauth.waiters[id]
	msgauth.Unlock()
	if !ok {
		return fmt.Errorf("no waiter for code %d", id)
	}
	ch <- from
	return nil
}

func EmailHandler(from string, rcpts []string, msg *mail.Message) {
	log.Printf("email from %q to %q subject %q", from, rcpts, msg.Header.Get("subject"))

	// handle msgauthwait emails.
	isMsgwait := false
	for _, r := range rcpts {
		if isMsgwait = strings.HasPrefix(r, "msgauth@"); isMsgwait {
			break
		}
	}
	if isMsgwait {
		if err := handleMsgauthEmail(from, msg); err != nil {
			log.Printf("msgauth@ email error: %v", err)
		}
	}
}
