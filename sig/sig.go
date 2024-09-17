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
// there are two available operations (both must be POST):
//
//   - /sig?get=$name[&timeoutms=*]: gets the content of a signal.
//     if timeoutms is positive, it waits for a signal to be posted up to for a timeoutms duration (default timeout is 0).
//   - /sig?set=$name[&timeoutms=*]: uploads the content of a signal.
//     blocks until another client gets the content up to the optional timeout (default timeout is 20 minutes).
//
// the requests return 204 on a timeout.
//
// example usage:
//
//	client 1: curl -X POST 'https://iio.ie/sig?get=examplename&timeoutms=600000'
//	client 2: curl -X POST 'https://iio.ie/sig?set=examplename' -d 'example content'
//
// client 1 will block until client 2 uploads their value.
//
// see https://iio.ie/webchat on how to use this.
package sig

import (
	"blog/limiter"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var mu sync.Mutex
var signals = map[string]*signal{}
var active = limiter.NewActiveLimiter(4000)

type signal struct {
	ch       chan []byte
	setters  atomic.Int32
	refcount int
}

func respond(w http.ResponseWriter, code int, format string, args ...any) {
	log.Printf("sig response: %s: %s", http.StatusText(code), fmt.Sprintf(format, args...))
	w.WriteHeader(code)
	fmt.Fprintf(w, format, args...)
}

func HandleHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if req.Method != "POST" {
		respond(w, http.StatusBadRequest, "must be POST")
		return
	}

	// read body early so that ParseForm doesn't eat it.
	if req.ContentLength == -1 {
		respond(w, http.StatusBadRequest, "missing content-length header")
		return
	}
	if req.ContentLength > 1e5 {
		respond(w, http.StatusBadRequest, "content-length too large")
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		respond(w, http.StatusBadRequest, "read body: %v", err)
		return
	}

	// sanity check the parameters.
	var name string
	var timeoutms int
	if err := req.ParseForm(); err != nil {
		respond(w, http.StatusBadRequest, "parse url: %v", err)
		return
	}
	if req.Form.Has("get") && req.Form.Has("set") {
		respond(w, http.StatusBadRequest, "must be either get or set")
		return
	}
	name = req.Form.Get("get") + req.Form.Get("set")
	if name == "" {
		respond(w, http.StatusBadRequest, "missing get or set")
		return
	}
	if req.Form.Has("set") {
		timeoutms = 20 * 60 * 1000
	}
	if len(name) > 64 {
		respond(w, http.StatusBadRequest, "name too long")
		return
	}
	if t := req.Form.Get("timeoutms"); len(t) > 0 {
		var err error
		timeoutms, err = strconv.Atoi(t)
		if err != nil {
			respond(w, http.StatusBadRequest, "parse timeoutms: %v", err)
			return
		}
	}
	if timeoutms < 0 || timeoutms > 20*60*1000 {
		w.WriteHeader(http.StatusBadRequest)
		respond(w, http.StatusBadRequest, "timeoutms out of range, max duration is 20 minutes")
		return
	}

	if !active.Add() {
		respond(w, http.StatusServiceUnavailable, "signaling service overloaded")
		return
	}
	defer active.Finish()

	mu.Lock()
	sig := signals[name]
	if sig == nil {
		sig = &signal{ch: make(chan []byte), refcount: 1}
		signals[name] = sig
	} else {
		sig.refcount++
	}
	mu.Unlock()

	if req.Form.Has("set") {
		if !sig.setters.CompareAndSwap(0, 1) {
			respond(w, http.StatusConflict, "signal %q has a pending setter already", name)
		} else {
			select {
			case sig.ch <- body:
				sig.setters.Store(0)
				respond(w, http.StatusOK, "ok")
			case <-req.Context().Done():
				sig.setters.Store(0)
				respond(w, http.StatusBadRequest, "set request for %q cancelled", name)
			case <-time.NewTimer(time.Duration(timeoutms) * time.Millisecond).C:
				sig.setters.Store(0)
				respond(w, http.StatusNoContent, "set request for %q timed out", name)
			}
		}
	} else if req.Form.Has("get") {
		select {
		case content := <-sig.ch:
			http.ServeContent(w, req, "sig", time.Time{}, bytes.NewReader(content))
		case <-req.Context().Done():
			respond(w, http.StatusBadRequest, "get request for %q cancelled", name)
		case <-time.NewTimer(time.Duration(timeoutms) * time.Millisecond).C:
			respond(w, http.StatusNoContent, "get request for %q timed out", name)
		}
	}

	mu.Lock()
	sig.refcount--
	if sig.refcount == 0 {
		delete(signals, name)
	}
	mu.Unlock()
}
