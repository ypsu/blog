package userapi

import (
	"blog/abname"
	"blog/alogdb"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	DefaultDB.Init()

	var lastStatus, lastResponse, lastCookie string
	call := func(body string) string {
		req := httptest.NewRequest("POST", "/userapi", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", lastCookie)
		resp := httptest.NewRecorder()
		DefaultDB.HandleHTTP(resp, req)
		rbody := efft.Must1(io.ReadAll(resp.Result().Body))
		lastStatus = resp.Result().Status
		lastResponse = strings.TrimSpace(string(rbody))
		if len(resp.Result().Cookies()) > 0 {
			c := resp.Result().Cookies()[0]
			lastCookie = c.Name + "=" + c.Value
		}
		return lastResponse
	}

	efft.Effect(call("")).Equals("userapi.EmptyAction (missing POST body?)")
	efft.Effect(call("action=dummy")).Equals("userapi.InvalidAction action=dummy")
	call("action=registerguest")
	efft.Effect(lastStatus).Equals("200 OK")
	efft.Effect(call("action=register")).Equals("userapi.UsernameOrPasswordMissing")
	efft.Effect(call("action=register&username=test-guest&password=testpassword")).Equals("userapi.InvalidUsernameCharacter username=\"test-guest\" (must be [a-z]+)")
	efft.Effect(call("action=register&username=testuser&password=testpassword&pubnote=Hello!&privnote=hello@example.com")).Equals("ok")
	regCookie := lastCookie
	efft.Effect(lastCookie).Equals("session=testuser.fcb6b0ccdd2e6326555ef6209e29e650c3a0eb6223b261bb1d51479ff1a7b470.babadabc")
	efft.Effect(call("action=login&username=baduser&password=testpassword")).Equals("userapi.LoginUsernameNotFound")
	efft.Effect(call("action=login&username=testuser&password=badpassword")).Equals("userapi.BadPassword")
	efft.Effect(call("action=login&username=testuser&password=testpassword")).Equals(`
		Hello!
		hello@example.com`)
	efft.Effect(lastCookie == regCookie).Equals("true")
	efft.Effect(call("action=logout")).Equals("ok")
	efft.Effect(call("action=logout")).Equals("userapi.NotLoggedIn")
	efft.Effect(call("action=login&username=testuser&password=testpassword")).Equals(`
		Hello!
		hello@example.com`)
	efft.Effect(lastCookie != regCookie).Equals("true")
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

	now := time.Date(2030, time.March, 1, 0, 0, 0, 0, time.UTC)
	efft.Effect(DefaultDB.Userinfo("aaxyzxyz-guest", now)).Equals("2025-January (5 years ago)")
	efft.Effect(DefaultDB.Userinfo("abxyzxyz-guest", now)).Equals("2025-February (5 years ago)")
	efft.Effect(DefaultDB.Userinfo("baxyzxyz-guest", now)).Equals("2027-March (3 years ago)")
	efft.Effect(DefaultDB.Userinfo("aaaxyzxyz-guest", now)).Equals("2025-January (5 years ago)")
	efft.Effect(DefaultDB.Userinfo("baaxyzxyz-guest", now)).Equals("2081-May (-614 months ago)")
	efft.Effect(DefaultDB.Userinfo("babxyzxyz-guest", now)).Equals("2081-June (-615 months ago)")
	efft.Effect(DefaultDB.Userinfo("testuser", now)).Equals(`
		1970-January (60 years ago)
		NewPubnote`)
	efft.Effect(DefaultDB.Userinfo("abc-guest", now)).Equals("userapi.BadGuestName")
	efft.Effect(DefaultDB.Userinfo("abc-foo", now)).Equals("")

	sessionsMap := map[abname.ID]uint64{}
	DefaultDB.userSessions.Range(func(key, value any) bool { sessionsMap[key.(abname.ID)] = value.(uint64); return true })
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

	DefaultDB.userSessions.Clear()
	DefaultDB.Init()

	sessionsMap = map[abname.ID]uint64{}
	DefaultDB.userSessions.Range(func(key, value any) bool { sessionsMap[key.(abname.ID)] = value.(uint64); return true })
	efft.Effect(userSessionsText == efft.Stringify(sessionsMap)).Equals("true")
}

func TestTenure(t *testing.T) {
	efft.Init(t)
	now := time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC)
	efft.Effect(tenure(now, now.AddDate(0, 0, -1))).Equals("2025-March (-1 months ago)")
	efft.Effect(tenure(now, now.AddDate(0, 0, 0))).Equals("2025-March (this month)")
	efft.Effect(tenure(now, now.AddDate(0, 0, 1))).Equals("2025-March (this month)")
	efft.Effect(tenure(now, now.AddDate(0, 1, 0))).Equals("2025-March (last month)")
	efft.Effect(tenure(now, now.AddDate(0, 3, 0))).Equals("2025-March (3 months ago)")
	efft.Effect(tenure(now, now.AddDate(0, 10, 0))).Equals("2025-March (10 months ago)")
	efft.Effect(tenure(now, now.AddDate(1, 0, 0))).Equals("2025-March (1 year ago)")
	efft.Effect(tenure(now, now.AddDate(3, 0, 0))).Equals("2025-March (3 years ago)")
}
