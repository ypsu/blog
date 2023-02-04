// package sig implement signaling helper service.
// that can be used to set up webrtc connections.
// it's not a production quality service
// but should be sufficient for oneoff demos.
//
// in this server's context a signal is a named, temporary message
// that can be read only once.
// after reading it, the server destroys the message.
// the content of such message is limited to be at most 100000 bytes long.
// a signal's name must match the "[a-z0-9]{1,15}" regex (without the quotes).
//
//   - /sig?op=get&name=[name][&blocking=1][&ifdiff=content]:
//     gets the content of a signal.
//   - /sig?op=put&name=[name]&value=[value][&blocking=1][&ifsame=content]:
//     uploads the content of a signal.
//
// keep in mind these restrictions:
//
//   - unset signals are treated as signals with empty content.
//     there's no difference between empty content and an unset signal.
//   - the contents is lost after 5 minutes of setting it.
//   - setting blocking=1 means the connection will hang
//     until the signal's value changes.
//   - ifdiff's value is what blocking=1 will wait to change from.
//     the default value is empty so by default /sig?op=get&blocking=1 will wait
//     until somebody sets a signal to a nonempty value.
//   - /sig?op=put can set the value conditionally with the ifsame parameter:
//     it will only set the value if it's the same as ifsame.
//     with blocking=1 it will wait until the signal will have the requested value.
//     ifsame is empty by default so a non-empty signal won't be overridden.
//     fetch the value first to override it.
//   - if there multiple readers blocked in /sig?op=get,
//     only the first one will get the value upon a write.
//     the second reader will need to wait for a new write.
//   - be warned: it's a bit tricky to use this right:
//     requires lot of careful edge case handling.
//
// example usage:
//
//   client 1: curl 'notech.ie:8080/sig?op=get&name=examplename&blocking=1'
//   client 2: curl 'notech.ie:8080/sig?op=put&name=examplename&value=example%20content%0a'
//
// client 1 will block until client 2 uploads their value.
//
// here's an example how a chat app could use this
// to establish a direct connection between the chat participants.
// assuming some familiarity with the webrtc offer/answer parlace.
//
//   - on pageload the page checks /sig?op=get&name=chatoffer for an offer.
//     if there's one, it accepts that and the page will be a client.
//   - otherwise it becomes a server and uploads an offer via /sig?op=put&name=chatoffer.
//     it then starts waiting for an answer via /sig?op=get&name=chatanswer&blocking=1.
//   - if the page is a client, then it returns an answer via /sig?op=put&name=chatanswer.
//   - both the client and server establish a webrtc connection with the data received.
//   - meanwhile the server uploads another offer via /sig?op=put&name=chatoffer
//     and waits for the next client via /sig?op=get&name=chatanswer&blocking=1.
//     this can be used to implement more than 2 participants.
package sig

import (
	"log"
	"net/http"
	"time"
)

func Init() {
	go main()
}

func HandleHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// sanity check the parameters.
	if req.ParseForm() != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: couldn't parse the url parameters\n"))
		return
	}
	params := req.Form
	op := params.Get("op")
	name := params.Get("name")
	if len(op) == 0 || len(name) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: missing op or name parameter\n"))
		return
	}
	if op != "get" && op != "put" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: op must be get or put\n"))
		return
	}
	if len(params.Get("value")) >= 100000 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 bad request: value too long\n"))
		return
	}

	// process the request in the main goroutine.
	done := make(chan struct{})
	requestq <- request{req: req, w: w, done: done}
	<-done
}

type request struct {
	// clearsignal if nonempty means that a signal should be checked for clearing.
	clearsignal string

	// if clearsignal is empty, it's a normal request.
	req  *http.Request
	w    http.ResponseWriter
	done chan<- struct{}
}

type signal struct {
	content string
	setTime time.Time
	waiters []request
}

var requestq = make(chan request, 10)

const timeout = 5 * time.Minute

var timers = 0

// processrequest true if the request was handled, can be deleted.
// returns false if it wasn't and must be kept around.
func processrequest(sig *signal, r request) bool {
	params := r.req.Form
	op := params.Get("op")
	name := params.Get("name")
	blocking := params.Get("blocking") == "1"

	if op == "get" {
		if sig.content != params.Get("ifdiff") {
			r.w.Header().Set("Content-Type", "text/plain")
			r.w.Write([]byte(sig.content))
			r.done <- struct{}{}
			sig.content = ""
			sig.setTime = time.Time{}
			return true
		} else if blocking {
			return false
		} else {
			r.w.WriteHeader(http.StatusPreconditionFailed)
			if len(params.Get("ifdiff")) == 0 {
				r.w.Write([]byte("412 precondition failed: content empty\n"))
			} else {
				r.w.Write([]byte("412 precondition failed: content already is the expected value\n"))
			}
			r.done <- struct{}{}
			return true
		}
	} else if op == "put" {
		if sig.content == params.Get("ifsame") {
			r.w.Write([]byte("ok\n"))
			r.done <- struct{}{}
			sig.content = params.Get("value")
			sig.setTime = time.Now()
			timers++
			if timers > 1000 {
				log.Fatal("too many concurrent timers")
			}
			go func() {
				time.Sleep(timeout)
				requestq <- request{clearsignal: name}
			}()
			return true
		} else if blocking {
			return false
		} else {
			r.w.WriteHeader(http.StatusPreconditionFailed)
			r.w.Write([]byte("412 precondition failed: content different\n"))
			r.done <- struct{}{}
			return true
		}
	}
	log.Fatal("unexpected codepath in sig")
	return true
}

func main() {
	signals := map[string]*signal{}
	for r := range requestq {
		var name string
		var sig *signal
		if len(r.clearsignal) > 0 {
			// process the clearsignal case.
			timers--
			name = r.clearsignal
			sig = signals[name]
			if sig == nil {
				continue
			}
			if time.Now().Sub(sig.setTime) >= timeout {
				sig.content = ""
				sig.setTime = time.Time{}
			}
		} else {
			// process the user request case.
			name = r.req.Form.Get("name")
			sig = signals[name]
			if sig == nil {
				log.Printf("creating signal %s", name)
				if len(signals) > 100 {
					log.Fatal("too many active signals")
				}
				sig = &signal{}
				signals[name] = sig
			}
			if !processrequest(sig, r) {
				if len(sig.waiters) > 20 {
					r.w.WriteHeader(http.StatusTooManyRequests)
					r.w.Write([]byte("429 too many requests: too many other clients waiting\n"))
					r.done <- struct{}{}
					continue
				}
				sig.waiters = append(sig.waiters, r)
			}
		}

		// now check if we we can unblock any waiters.
		changed := true
		for changed {
			changed = false
			for idx, waitreq := range sig.waiters {
				if waitreq.req.Context().Err() == nil && !processrequest(sig, waitreq) {
					continue
				}
				sig.waiters = append(sig.waiters[:idx], sig.waiters[idx+1:]...)
				changed = true
				break
			}
		}
		if sig.content == "" && len(sig.waiters) == 0 {
			log.Printf("clearing signal %s", name)
			delete(signals, name)
		}
	}
}
