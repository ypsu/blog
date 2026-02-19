package eventz

import (
	"fmt"
	"html"
	"log"
	"maps"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
)

// Overrideable for testing.
var now = time.Now

type eventstat struct {
	firstevent string
	lastevent  string
	count      int
}

type EventZ struct {
	mu     sync.Mutex
	events map[string]eventstat
}

var Default = New()

func New() *EventZ {
	ez := &EventZ{events: map[string]eventstat{}}
	ez.Printf("eventz.ServerStart")
	return ez
}

// Print logs a message to eventz.
// It uses the first field from it as the name of the message.
func (ez *EventZ) Print(event string) {
	name, event, _ := strings.Cut(event, " ")
	now := now()
	event = fmt.Sprintf("%02d%02d%02d.%02d%02d%02dz %s", now.Year()%100, now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), event)
	log.Printf("eventz.Print: %s %s", name, event)

	ez.mu.Lock()
	defer ez.mu.Unlock()
	ms := ez.events[name]
	if ms.count == 0 {
		ms.firstevent = event
	}
	ms.lastevent = event
	ms.count++
	ez.events[name] = ms
}

// Printf logs a message to eventz.
// Printf formats the message and then uses the first field from it as the name of the message.
func (ez *EventZ) Printf(format string, a ...any) {
	ez.Print(fmt.Sprintf(format, a...))
}

// ServeHTTP prints the active messages and allows clearing them too.
// Should be available only for administrators.
func (ez *EventZ) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ez.mu.Lock()
	defer ez.mu.Unlock()

	clearuntil := r.FormValue("clearuntil")
	if clearuntil == "" {
		fmt.Fprint(w, responsePrefix)
		keys, lastt := slices.Sorted(maps.Keys(ez.events)), ""
		for _, k := range keys {
			ms := ez.events[k]
			t, _, _ := strings.Cut(ms.lastevent, " ")
			lastt = max(lastt, t)
			if ms.count == 1 {
				fmt.Fprintf(w, "%s: %s\n", k, html.EscapeString(ms.firstevent))
			} else {
				fmt.Fprintf(w, "%s count=%d\n  first: %s\n  last:  %s\n", k, ms.count, html.EscapeString(ms.firstevent), html.EscapeString(ms.lastevent))
			}
		}
		version := "unknown"
		if bi, ok := debug.ReadBuildInfo(); ok {
			version = bi.Main.Version
		}
		fmt.Fprintf(w, responseSuffix, version, lastt)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "eventz.BadMethod", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(clearuntil, "2") {
		http.Error(w, "eventz.BadClearBeforeValue", http.StatusBadRequest)
		return
	}
	for k, ms := range ez.events {
		mt, _, _ := strings.Cut(ms.lastevent, " ")
		if mt <= clearuntil {
			delete(ez.events, k)
		}
	}
}

const responsePrefix = "<!doctype html><title>eventz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>\n"
const responseSuffix = "</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><pre>ServerVersion: %s</pre><script>let LastT = %q</script><script type=module src=eventz.js></script>\n"
