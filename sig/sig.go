// package sig implement signaling helper service.
// that can be used to set up webrtc connections.
// it's not a production quality service
// but should be sufficient for oneoff demos.
//
// in this server's context a signal is a named, temporary message
// that can be read only once.
// after reading it, the server destroys the message.
// the content of such message is limited to be at most 100000 bytes long.
// a signal's name must match the "[a-z][a-z0-9_]{1,31}" regex (without the quotes).
// the operation depends on the http method used:
//
//   - GET: /sig?name=*[&timeoutms=*]: gets the content of a signal.
//     if timeoutms is positive, it waits for a signal to be posted up to for a timeoutms duration.
//   - POST: /sig?name=[name]: uploads the content of a signal.
//     blocks until another client gets the content.
//
// the requests return 204 on a timeout.
// e.g. a GET without timeoutms for a non-existent signal will return 204 immediately.
//
// example usage:
//
//	client 1: curl 'notech.ie/sig?name=examplename&timeoutms=600000'
//	client 2: curl 'notech.ie/sig?name=examplename' -X POST -d 'example content'
//
// client 1 will block until client 2 uploads their value.
//
// see https://notech.ie/webchat on how to use this.
package sig

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var mu sync.Mutex
var signals = map[string]*signal{}

type signal struct {
	ch       chan []byte
	refcount int
}

func HandleHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// read body early so that ParseForm doesn't eat it.
	if req.ContentLength == -1 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: missing content-length header\n"))
		return
	}
	if req.ContentLength > 1e5 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: content-length too large\n"))
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400 bad request: reading body: %v\n", err)
		return
	}

	// sanity check the parameters.
	var name string
	var timeoutms int
	if req.ParseForm() != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: couldn't parse the url parameters\n"))
		return
	}
	name = req.Form.Get("name")
	if len(name) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: missing name parameter\n"))
		return
	}
	if len(name) > 64 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: name parameter too long\n"))
		return
	}
	if t := req.Form.Get("timeoutms"); len(t) > 0 {
		var err error
		timeoutms, err = strconv.Atoi(t)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "400 bad request: bad timeoutms param: %v\n", err)
			return
		}
	}
	if timeoutms < 0 || timeoutms > 20*60*1000 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: timeoutms out of range, max duration is 20 minutes\n"))
		return
	}

	mu.Lock()
	sig := signals[name]
	if sig == nil {
		sig = &signal{ch: make(chan []byte), refcount: 1}
		signals[name] = sig
	} else {
		sig.refcount++
	}
	mu.Unlock()

	if req.Method == "POST" {
		log.Printf("posting signal for %q", name)
		select {
		case sig.ch <- body:
			w.Write([]byte("ok\n"))
			log.Printf("successful signal forward for %q", name)
		case <-req.Context().Done():
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "400 bad request: request cancelled: %v\n", req.Context().Err())
			log.Printf("post cancelled for signal %q", name)
		case <-time.NewTimer(20 * time.Minute).C:
			w.WriteHeader(http.StatusNoContent)
			w.Write([]byte("204 no content: request timed out\n"))
			log.Printf("post timed out of signal %q", name)
		}
	} else if req.Method == "GET" {
		log.Printf("getting signal %q with timeoutms %d", name, timeoutms)
		select {
		case content := <-sig.ch:
			http.ServeContent(w, req, "sig", time.Time{}, bytes.NewReader(content))
		case <-req.Context().Done():
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "400 bad request: request cancelled: %v\n", req.Context().Err())
			log.Printf("get cancelled for signal %q", name)
		case <-time.NewTimer(time.Duration(timeoutms) * time.Millisecond).C:
			w.WriteHeader(http.StatusNoContent)
			w.Write([]byte("204 no content: request timed out\n"))
			log.Printf("get timed out for signal %q", name)
		}
	}

	mu.Lock()
	sig.refcount--
	if sig.refcount == 0 {
		delete(signals, name)
	}
	mu.Unlock()
}
