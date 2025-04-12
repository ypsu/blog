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

	"github.com/ypsu/efftesting"
)

func TestAPI(t *testing.T) {
	et := efftesting.New(t)
	logfile := filepath.Join(t.TempDir(), "log")
	fh := efftesting.Must1(os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0644))
	defer fh.Close()
	efftesting.Override(&alogdb.DefaultDB, efftesting.Must1(alogdb.NewForTesting(fh)))
	efftesting.Override(&alogdb.Now, func() int64 { return 1 })
	var randomValue uint64 = 0xbabadaba
	efftesting.Override(&rand64, func() uint64 { randomValue++; return randomValue })
	efftesting.Override(&randsalt, func() string { randomValue++; return fmt.Sprintf("RANDSALT%x", randomValue) })
	abname.Init()
	DefaultDB.Init()

	var lastStatus, lastResponse, lastCookie string
	call := func(body string) string {
		req := httptest.NewRequest("POST", "/userapi", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", lastCookie)
		resp := httptest.NewRecorder()
		DefaultDB.HandleHTTP(resp, req)
		rbody, err := io.ReadAll(resp.Result().Body)
		efftesting.Must(err)
		lastStatus = resp.Result().Status
		lastResponse = strings.TrimSpace(string(rbody))
		if len(resp.Result().Cookies()) > 0 {
			c := resp.Result().Cookies()[0]
			lastCookie = c.Name + "=" + c.Value
		}
		return lastResponse
	}

	et.Expect("EmptyCall", call(""), "userapi.EmptyAction (missing POST body?)")
	et.Expect("DummyAction", call("action=dummy"), "userapi.InvalidAction action=dummy")
	call("action=registerguest")
	et.Expect("RegisterGuest", lastStatus, "200 OK")
	et.Expect("RegisterWithEmptyForm", call("action=register"), "userapi.UsernameOrPasswordMissing")
	et.Expect("RegisterBadName", call("action=register&username=test-guest&password=testpassword"), "userapi.InvalidUsernameCharacter username=\"test-guest\" (must be [a-z]+)")
	et.Expect("Register", call("action=register&username=testuser&password=testpassword&pubnote=Hello!&privnote=hello@example.com"), "ok")
	regCookie := lastCookie
	et.Expect("RegistrationCookie", lastCookie, "session=testuser fcb6b0ccdd2e6326555ef6209e29e650c3a0eb6223b261bb1d51479ff1a7b470 babadabc")
	et.Expect("LoginBadUser", call("action=login&username=baduser&password=testpassword"), "userapi.LoginUsernameNotFound")
	et.Expect("LoginBadPassword", call("action=login&username=testuser&password=badpassword"), "userapi.BadPassword")
	et.Expect("LoginOK", call("action=login&username=testuser&password=testpassword"), `
		Hello!
		hello@example.com`)
	et.Expect("LoginCookieIsSame", lastCookie == regCookie, "true")
	et.Expect("Logout", call("action=logout"), "ok")
	et.Expect("BadLogout", call("action=logout"), "userapi.NotLoggedIn")
	et.Expect("Login2OK", call("action=login&username=testuser&password=testpassword"), `
		Hello!
		hello@example.com`)
	et.Expect("Login2CookieIsDifferent", lastCookie != regCookie, "true")
	et.Expect("Userdata", call("action=userdata"), `
		Hello!
		hello@example.com`)

	et.Expect("IneffectiveUpdate", call("action=update&pubnote=Hello!"), "ok")
	et.Expect("UpdateBothNotes", call("action=update&pubnote=NewPubnote&privnote=NewPrivnote"), "ok")
	et.Expect("UpdatePasswordBad", call("action=update&oldpassword=blah&newpassword=newpassword"), "userapi.BadOldPassword")
	et.Expect("UpdatePasswordOK", call("action=update&oldpassword=testpassword&newpassword=newpassword"), "ok")
	et.Expect("LogoutAfterPasswordChange", call("action=logout"), "ok")
	et.Expect("LoginAfterPasswordChange", call("action=login&username=testuser&password=newpassword"), `
		NewPubnote
		NewPrivnote`)

	now := time.Date(2030, time.March, 1, 0, 0, 0, 0, time.UTC)
	et.Expect("GuestUserinfo1", DefaultDB.Userinfo("aaxyzxyz-guest", now), "2025-January (5 years ago)")
	et.Expect("GuestUserinfo2", DefaultDB.Userinfo("abxyzxyz-guest", now), "2025-February (5 years ago)")
	et.Expect("GuestUserinfo3", DefaultDB.Userinfo("baxyzxyz-guest", now), "2027-March (3 years ago)")
	et.Expect("GuestUserinfo4", DefaultDB.Userinfo("aaaxyzxyz-guest", now), "2025-January (5 years ago)")
	et.Expect("GuestUserinfo5", DefaultDB.Userinfo("baaxyzxyz-guest", now), "2081-May (-614 months ago)")
	et.Expect("GuestUserinfo6", DefaultDB.Userinfo("babxyzxyz-guest", now), "2081-June (-615 months ago)")
	et.Expect("RegisteredUserinfo", DefaultDB.Userinfo("testuser", now), `
		1970-January (60 years ago)
		NewPubnote`)
	et.Expect("BadGuestUserinfo", DefaultDB.Userinfo("abc-guest", now), "userapi.BadGuestName")
	et.Expect("BadUserinfo", DefaultDB.Userinfo("abc-foo", now), "")

	sessionsMap := map[abname.ID]uint64{}
	DefaultDB.userSessions.Range(func(key, value any) bool { sessionsMap[key.(abname.ID)] = value.(uint64); return true })
	userSessionsText := efftesting.Stringify(sessionsMap)
	et.Expect("TestuserAbname", int64(efftesting.Must1(abname.New("testuser"))), "5115938644328448")
	et.Expect("SessionsMap", userSessionsText, `
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
	et.Expect("BackendContent", removeTS(strings.ReplaceAll(string(efftesting.Must1(os.ReadFile(logfile))), "\000", "")), `
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
	et.Expect("LoadSessionsIsSame", userSessionsText == efftesting.Stringify(sessionsMap), "true")
}

func TestTenure(t *testing.T) {
	et := efftesting.New(t)
	now := time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC)
	et.Expect("Future", tenure(now, now.AddDate(0, 0, -1)), "2025-March (-1 months ago)")
	et.Expect("Today", tenure(now, now.AddDate(0, 0, 0)), "2025-March (this month)")
	et.Expect("Tomorrow", tenure(now, now.AddDate(0, 0, 1)), "2025-March (this month)")
	et.Expect("NextMonth", tenure(now, now.AddDate(0, 1, 0)), "2025-March (last month)")
	et.Expect("ThreeMonthsLater", tenure(now, now.AddDate(0, 3, 0)), "2025-March (3 months ago)")
	et.Expect("NextJanuary", tenure(now, now.AddDate(0, 10, 0)), "2025-March (10 months ago)")
	et.Expect("NextYear", tenure(now, now.AddDate(1, 0, 0)), "2025-March (1 year ago)")
	et.Expect("ThreeYearsLater", tenure(now, now.AddDate(3, 0, 0)), "2025-March (3 years ago)")
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
