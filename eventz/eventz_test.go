package eventz

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ypsu/efftesting/efft"
)

func TestEventZ(t *testing.T) {
	efft.Init(t)

	tm := time.Date(2025, 1, 30, 23, 30, 0, 0, time.UTC)
	efft.Override(&now, func() time.Time {
		tm = tm.Add(time.Minute)
		return tm
	})

	ez := New()
	ez.Printf("HelloWorld name=%s", "alice")
	ez.Printf("HelloWorld name=%s", "bob")
	ez.Printf("HelloWorld name=%s", "charlie")
	ez.Printf("HelloWorld name=%s", "dave")
	ez.Printf("UniqueHello name=%s", "bob")

	request := httptest.NewRequest("GET", "/eventz", nil)
	response := httptest.NewRecorder()
	ez.ServeHTTP(response, request)
	efft.Effect(response.Body).Equals(`
		<!doctype html><title>eventz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>
		HelloWorld count=4
		  first: 250130.233200z name=alice
		  last:  250130.233500z name=dave
		UniqueHello: 250130.233600z name=bob
		eventz.ServerStart: 250130.233100z 
		</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><pre>ServerVersion: (devel)</pre><script>let LastT = "250130.233600z"</script><script type=module src=eventz.js></script>
	`)

	request = httptest.NewRequest("GET", "/eventz?clearuntil=250130.2335", nil)
	response = httptest.NewRecorder()
	ez.ServeHTTP(response, request)
	efft.Effect(strings.TrimSpace(response.Body.String())).Equals("eventz.BadMethod")

	request = httptest.NewRequest("POST", "/eventz?clearuntil=blah", nil)
	response = httptest.NewRecorder()
	ez.ServeHTTP(response, request)
	efft.Effect(strings.TrimSpace(response.Body.String())).Equals("eventz.BadClearBeforeValue")

	request = httptest.NewRequest("POST", "/eventz?clearuntil=250130.233500z", nil)
	response = httptest.NewRecorder()
	ez.ServeHTTP(response, request)
	efft.Effect(response.Body).Equals("")

	request = httptest.NewRequest("GET", "/eventz", nil)
	response = httptest.NewRecorder()
	ez.ServeHTTP(response, request)
	efft.Effect(response.Body).Equals(`
		<!doctype html><title>eventz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>
		UniqueHello: 250130.233600z name=bob
		</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><pre>ServerVersion: (devel)</pre><script>let LastT = "250130.233600z"</script><script type=module src=eventz.js></script>
	`,
	)
}
