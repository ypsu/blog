package userapi

import (
	"blog/abname"
	"blog/alogdb"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ypsu/efftesting/efft"
)

func TestAPI(t *testing.T) {
	efft.Init(t)
	logfile := filepath.Join(t.TempDir(), "log")
	fh := efft.Must1(os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0644))
	defer fh.Close()
	efft.Override(&alogdb.DefaultDB, efft.Must1(alogdb.NewForTesting(fh)))
	efft.Override(&alogdb.Now, func() int64 { return 1 })
	var randomValue uint64 = 0xbabadaba
	efft.Override(&rand64, func() uint64 { randomValue++; return randomValue })
	efft.Override(&randsalt, func() string { randomValue++; return fmt.Sprintf("RANDSALT%x", randomValue) })
	abname.Init()
	var db DB
	db.Init()

	var lastStatus string
	var cookies []*http.Cookie
	call := func(body string) string {
		req := httptest.NewRequest("POST", "/userapi", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		username, _, _ := db.Session(req)
		resp := httptest.NewRecorder()
		db.HandleHTTP(resp, req, username, true)
		lastStatus = resp.Result().Status
		if cs := resp.Result().Cookies(); len(cs) != 0 {
			cookies = cs
		}
		rbody := efft.Must1(io.ReadAll(resp.Result().Body))
		return strings.TrimSpace(string(rbody))
	}

	efft.Effect(call("")).Equals("userapi.EmptyAction (missing POST body?)")
	efft.Effect(call("action=dummy")).Equals("userapi.InvalidAction action=dummy")
	call("action=registerguest")
	efft.Effect(lastStatus).Equals("200 OK")
	efft.Effect(call("action=register")).Equals("userapi.UsernameOrPasswordMissing")
	efft.Effect(call("action=register&username=test-guest&password=testpassword")).Equals("userapi.InvalidUsernameCharacter username=\"test-guest\" (must be [a-z]+)")
	efft.Effect(call("action=register&username=testuser&password=testpassword&pubnote=Hello!&privnote=hello@example.com")).Equals("ok")
	regCookie := cookies
	efft.Effect(cookies[0].Name + "=" + cookies[0].Value).Equals("session=testuser.fcb6b0ccdd2e6326555ef6209e29e650c3a0eb6223b261bb1d51479ff1a7b470.babadabc")
	efft.Effect(cookies[1].Name + "=" + cookies[1].Value).Equals("username=testuser")
	efft.Effect(call("action=login&username=baduser&password=testpassword")).Equals("userapi.LoginUsernameNotFound")
	efft.Effect(call("action=login&username=testuser&password=badpassword")).Equals("userapi.BadPassword")
	efft.Effect(call("action=login&username=testuser&password=testpassword")).Equals(`
		Hello!
		hello@example.com`)
	efft.Effect(efft.Stringify(cookies) == efft.Stringify(regCookie)).Equals("true")
	efft.Effect(call("action=logout")).Equals("ok")
	efft.Effect(call("action=logout")).Equals("userapi.NotLoggedIn")
	efft.Effect(call("action=login&username=testuser&password=testpassword")).Equals(`
		Hello!
		hello@example.com`)
	efft.Effect(efft.Stringify(cookies) != efft.Stringify(regCookie)).Equals("true")
	efft.Effect(call("action=userdata")).Equals(`
		Hello!
		hello@example.com`)

	efft.Effect(call("action=update&pubnote=Hello!")).Equals("ok")
	efft.Effect(call("action=update&pubnote=NewPubnote&privnote=NewPrivnote")).Equals("ok")
	efft.Effect(call("action=update&oldpassword=blah&newpassword=newpassword")).Equals("userapi.BadOldPassword")
	efft.Effect(call("action=update&oldpassword=testpassword&newpassword=newpassword")).Equals("ok")
	efft.Effect(call("action=logout")).Equals("ok")
	efft.Effect(call("action=login&username=testuser&password=newpassword")).Equals(`
		NewPubnote
		NewPrivnote`)

	sessionsMap := map[abname.ID]uint64{}
	db.userSessions.Range(func(key, value any) bool { sessionsMap[key.(abname.ID)] = value.(uint64); return true })
	userSessionsText := efft.Stringify(sessionsMap)
	efft.Effect(int64(efft.Must1(abname.New("testuser")))).Equals("5115938644328448")
	efft.Effect(userSessionsText).Equals(`
		{
		  "5115938644328448": 3132807871
		}`)

	removeTS := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			if line != "" {
				lines[i] = strings.Join(strings.Fields(line)[1:], " ")
			}
		}
		return strings.Join(lines, "\n")
	}
	efft.Effect(removeTS(strings.ReplaceAll(string(efft.Must1(os.ReadFile(logfile))), "\000", ""))).Equals(`
		userapi.testuser register
		userapi.testuser pwhash RANDSALTbabadabb 8ff4a2ea41323e83ba91a8e0a9adc6f2cdd5d248dd39f3a86225eb9cc5c05320
		userapi.testuser pubnote Hello!
		userapi.testuser privnote hello@example.com
		usersessions testuser babadabc
		usersessions testuser 0
		usersessions testuser babadabd
		userapi.testuser pubnote NewPubnote
		userapi.testuser privnote NewPrivnote
		userapi.testuser pwhash RANDSALTbabadabe c1f5f925ef87959fd1322cb1a1c42ff69ae0ead095e80a6a301b4248f28c7271
		usersessions testuser 0
		usersessions testuser babadabf
	`)

	db.userSessions.Clear()
	db.Init()

	sessionsMap = map[abname.ID]uint64{}
	db.userSessions.Range(func(key, value any) bool { sessionsMap[key.(abname.ID)] = value.(uint64); return true })
	efft.Effect(userSessionsText == efft.Stringify(sessionsMap)).Equals("true")
}
