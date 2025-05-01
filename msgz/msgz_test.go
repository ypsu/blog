package msgz

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ypsu/efftesting"
)

func TestMsgZ(t *testing.T) {
	et := efftesting.New(t)

	tm := time.Date(2025, 1, 30, 23, 30, 0, 0, time.UTC)
	efftesting.Override(&now, func() time.Time {
		tm = tm.Add(time.Minute)
		return tm
	})

	mz := Init()
	mz.Printf("HelloWorld name=%s", "alice")
	mz.Printf("HelloWorld name=%s", "bob")
	mz.Printf("HelloWorld name=%s", "charlie")
	mz.Printf("HelloWorld name=%s", "dave")
	mz.Printf("UniqueHello name=%s", "bob")

	request := httptest.NewRequest("GET", "/msgz", nil)
	response := httptest.NewRecorder()
	mz.HandleMsgZ(response, request)
	et.Expect("Lookup", response.Body, `
		<!doctype html><title>msgz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>
		HelloWorld count=4
		  first: 250130.233200z name=alice
		  last:  250130.233500z name=dave
		UniqueHello: 250130.233600z name=bob
		msgz.ServerStart: 250130.233100z 
		</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><script>let LastT = "250130.233600z"</script><script type=module src=msgz.js></script>
	`)

	request = httptest.NewRequest("GET", "/msgz?clearuntil=250130.2335", nil)
	response = httptest.NewRecorder()
	mz.HandleMsgZ(response, request)
	et.Expect("BadClearMethod", strings.TrimSpace(response.Body.String()), "msgz.BadMethod")

	request = httptest.NewRequest("POST", "/msgz?clearuntil=blah", nil)
	response = httptest.NewRecorder()
	mz.HandleMsgZ(response, request)
	et.Expect("BadClearArgument", strings.TrimSpace(response.Body.String()), "msgz.BadClearBeforeValue")

	request = httptest.NewRequest("POST", "/msgz?clearuntil=250130.233500z", nil)
	response = httptest.NewRecorder()
	mz.HandleMsgZ(response, request)
	et.Expect("Clear", response.Body, "")

	request = httptest.NewRequest("GET", "/msgz", nil)
	response = httptest.NewRecorder()
	mz.HandleMsgZ(response, request)
	et.Expect("LookupAfterClear", response.Body, `
		<!doctype html><title>msgz</title><meta name=viewport content='width=device-width,initial-scale=1'><style>:root{color-scheme:light dark}</style><link rel=stylesheet href=style.css><pre id=ePre>
		UniqueHello: 250130.233600z name=bob
		</pre><button id=eButton>Clear until xxx</button><pre id=eError class=cbgNegative hidden></pre><script>let LastT = "250130.233600z"</script><script type=module src=msgz.js></script>
	`,
	)
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
