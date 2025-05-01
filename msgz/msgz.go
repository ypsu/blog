package msgz

import (
	"fmt"
	"html"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// Overrideable for testing.
var now = time.Now

type msgstat struct {
	firstmsg string
	lastmsg  string
	count    int
}

type MsgZ struct {
	mu   sync.Mutex
	msgs map[string]msgstat
}

var Default = Init()

func Init() *MsgZ {
	mz := &MsgZ{msgs: map[string]msgstat{}}
	mz.Printf("msgz.ServerStart")
	return mz
}

// Print logs a message to msgz.
// It uses the first field from it as the name of the message.
func (mz *MsgZ) Print(msg string) {
	name, msg, _ := strings.Cut(msg, " ")
	now := now()
	msg = fmt.Sprintf("%02d%02d%02d.%02d%02d%02dz %s", now.Year()%100, now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), msg)
	log.Printf("msgz.Print: %s %s", name, msg)

	mz.mu.Lock()
	defer mz.mu.Unlock()
	ms := mz.msgs[name]
	if ms.count == 0 {
		ms.firstmsg = msg
	}
	ms.lastmsg = msg
	ms.count++
	mz.msgs[name] = ms
}

// Printf logs a message to msgz.
// Printf formats the message and then uses the first field from it as the name of the message.
func (mz *MsgZ) Printf(format string, a ...any) {
	mz.Print(fmt.Sprintf(format, a...))
}

// HandleMsgZ prints the active messages and allows clearing them too.
// Should be available only for administrators.
func (mz *MsgZ) HandleMsgZ(w http.ResponseWriter, r *http.Request) {
	mz.mu.Lock()
	defer mz.mu.Unlock()

	clearuntil := r.FormValue("clearuntil")
	if clearuntil == "" {
		fmt.Fprint(w, responsePrefix)
		keys, lastt := slices.Sorted(maps.Keys(mz.msgs)), ""
		for _, k := range keys {
			ms := mz.msgs[k]
			t, _, _ := strings.Cut(ms.lastmsg, " ")
			lastt = max(lastt, t)
			if ms.count == 1 {
				fmt.Fprintf(w, "%s: %s\n", k, html.EscapeString(ms.firstmsg))
			} else {
				fmt.Fprintf(w, "%s count=%d\n  first: %s\n  last:  %s\n", k, ms.count, html.EscapeString(ms.firstmsg), html.EscapeString(ms.lastmsg))
			}
		}
		fmt.Fprintf(w, responseSuffix, lastt)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "msgz.BadMethod", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(clearuntil, "2") {
		http.Error(w, "msgz.BadClearBeforeValue", http.StatusBadRequest)
		return
	}
	for k, ms := range mz.msgs {
		mt, _, _ := strings.Cut(ms.lastmsg, " ")
		if mt <= clearuntil {
			delete(mz.msgs, k)
		}
	}
}

const responsePrefix = "<!doctype html><title>msgz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>\n"
const responseSuffix = "</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><script>let LastT = %q</script><script type=module src=msgz.js></script>\n"
