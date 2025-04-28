// Package userapi manages user registrations and logins.
package userapi

import (
	"blog/abname"
	"blog/alogdb"
	"blog/msgz"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	pseudorand "math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const guestIDLen = 6 // this is the random part, the timestamp is not included

var salt = os.Getenv("SALT")

type DB struct {
	mu      sync.Mutex
	lastreg time.Time

	userSessions sync.Map // should be map[abname.ID]uint64
}

var DefaultDB = DB{}

// Overrideable for testing.
var now = func() time.Time { return time.Now() }

func (db *DB) Init() {
	if salt == "" {
		log.Printf("userapi.NoSalt")
		msgz.Default.Printf("userapi.NoSalt")
	}

	for _, e := range alogdb.DefaultDB.Get("usersessions") {
		var user string
		var sid uint64
		fmt.Sscanf(e.Text, "%s %x", &user, &sid)
		uid, _ := abname.New(user)
		if sid == 0 {
			db.userSessions.Delete(uid)
		} else {
			db.userSessions.Store(uid, sid)
		}
	}
}

func (db *DB) HandleHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, fmt.Sprintf("userapi.InvalidMethod method=%s (must be POST)", req.Method), http.StatusMethodNotAllowed)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(w, "userapi.ParseForm: "+err.Error(), http.StatusBadRequest)
		return
	}

	action := req.FormValue("action")
	switch action {
	case "username":
		db.printUser(w, req)
	case "login":
		db.login(w, req)
	case "logout":
		db.logout(w, req)
	case "registerguest":
		db.registerGuest(w, req)
	case "register":
		db.registerFull(w, req)
	case "update":
		db.update(w, req)
	case "userdata":
		db.userdata(w, req)
	case "":
		http.Error(w, "userapi.EmptyAction (missing POST body?)", http.StatusBadRequest)
	default:
		http.Error(w, "userapi.InvalidAction action="+action, http.StatusBadRequest)
	}
}

func (db *DB) registerGuest(w http.ResponseWriter, req *http.Request) {
	if user := db.Username(w, req); user != "" {
		http.Error(w, user, http.StatusAlreadyReported)
		return
	}

	now, userid := now(), make([]byte, 0, 10)
	birthMonth := max(min((now.Year()-2025)*12+int(now.Month()-time.January), 26*26*26-1), 0)
	a, b, c := 'a'+byte(birthMonth/26/26), 'a'+byte(birthMonth/26%26), 'a'+byte(birthMonth%26)
	if a != 'a' {
		userid = append(userid, a)
	}
	userid = append(userid, b, c)
	for i := 0; i < guestIDLen; i++ {
		userid = append(userid, byte('a'+pseudorand.IntN(26)))
	}
	username := string(userid) + "-guest"
	tohash := username + " " + salt
	hash := sha256.Sum256([]byte(tohash))
	sig := hex.EncodeToString(hash[:])

	log.Printf("userapi.RegisteredGuest user=%s", username)
	msgz.Default.Printf("userapi.RegisteredGuest user=%s", username)
	w.Header().Add("Set-Cookie", fmt.Sprintf("session=%s.%s; Max-Age=2147483647; SameSite=Strict", username, sig))
	http.Error(w, username, http.StatusOK)
}

func hash(username, password, salt string) string {
	dk, err := pbkdf2.Key(sha256.New, username+password, []byte(salt), 1e5, 32)
	if err != nil {
		panic("userapi.PBKDF2: " + err.Error()) // should never happen
	}
	return hex.EncodeToString(dk)
}

var rand64 = func() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return binary.NativeEndian.Uint64(b[:])
}

var randsalt = func() string { return rand.Text() }

func (db *DB) registerFull(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		http.Error(w, "userapi.BadContentType", http.StatusBadRequest)
		return
	}
	username, password := req.FormValue("username"), req.FormValue("password")
	if username == "" || password == "" {
		http.Error(w, "userapi.UsernameOrPasswordMissing", http.StatusBadRequest)
		return
	}
	pubnote, privnote := req.FormValue("pubnote"), req.FormValue("privnote")
	if strings.IndexByte(pubnote, 0) != -1 || strings.IndexByte(privnote, 0) != -1 {
		http.Error(w, "userapi.ZeroByteInNote", http.StatusBadRequest)
		return
	}
	if strings.IndexByte(pubnote, '\n') != -1 || strings.IndexByte(privnote, '\n') != -1 {
		http.Error(w, "userapi.NewlineInNote", http.StatusBadRequest)
		return
	}
	if len(pubnote) > 200 || len(privnote) > 200 {
		http.Error(w, "userapi.NoteTooLong", http.StatusBadRequest)
		return
	}
	if len(username) < 3 {
		http.Error(w, fmt.Sprintf("userapi.UsernameTooShort username=%q (must be at least 3 chars)", username), http.StatusBadRequest)
		return
	}
	if len(username) > 10 {
		http.Error(w, fmt.Sprintf("userapi.UsernameTooLong username=%q (must be at most 10 chars)", username), http.StatusBadRequest)
		return
	}
	for _, ch := range username {
		if ch < 'a' || 'z' < ch {
			http.Error(w, fmt.Sprintf("userapi.InvalidUsernameCharacter username=%q (must be [a-z]+)", username), http.StatusBadRequest)
			return
		}
	}
	uid, err := abname.New(username)
	if uid == 0 || err != nil {
		http.Error(w, fmt.Sprintf("userapi.EncodeUsername username=%q: %v", username, err), http.StatusBadRequest)
	}
	msgz.Default.Printf("userapi.RegisterAttempt username=%q", username)

	db.mu.Lock()
	defer db.mu.Unlock()

	dbname := "userapi." + username
	entries := alogdb.DefaultDB.Get(dbname)
	if len(entries) != 0 {
		log.Printf("userapi.RegisterTakenUsername username=%s", username)
		http.Error(w, fmt.Sprintf("userapi.UsernameTaken username=%q", username), http.StatusConflict)
		return
	}

	if time.Since(db.lastreg) < time.Minute {
		log.Printf("userapi.TooManyRegistrations username=%s", username)
		http.Error(w, "userapi.TooManyRegistrations (try a minute later)", http.StatusServiceUnavailable)
		return
	}

	db.lastreg = time.Now()
	pwsalt, session := randsalt(), rand64()
	items := []string{
		"register", // needed for time tracking in case the entries below get garbage collected
		"pwhash " + pwsalt + " " + hash(username, password, pwsalt),
	}
	if pubnote != "" {
		items = append(items, "pubnote "+pubnote)
	}
	if privnote != "" {
		items = append(items, "privnote "+privnote)
	}
	if _, err := alogdb.DefaultDB.Add(dbname, items...); err != nil {
		http.Error(w, "userapi.AddRegistration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ignore login error: at worst the user needs to relogin later.
	alogdb.DefaultDB.Add("usersessions", username+" "+strconv.FormatUint(session, 16))

	db.userSessions.Store(uid, session)
	sids := strconv.FormatUint(session, 16)
	tohash := username + " " + salt + " " + sids
	hash := sha256.Sum256([]byte(tohash))
	sig := hex.EncodeToString(hash[:])
	w.Header().Add("Set-Cookie", fmt.Sprintf("session=%s.%s.%s; Max-Age=2147483647; SameSite=Strict", username, sig, sids))
	http.Error(w, "ok", http.StatusOK)
	log.Printf("userapi.UserRegistered username=%s", username)
	msgz.Default.Printf("userapi.RegisteredUser username=%s", username)
}

func (db *DB) userdata(w http.ResponseWriter, req *http.Request) {
	username := db.Username(w, req)
	if username == "" {
		http.Error(w, "userapi.NotLoggedIn", http.StatusPreconditionRequired)
		return
	}
	var pubnote, privnote string
	for _, e := range alogdb.DefaultDB.Get("userapi." + username) {
		if note, found := strings.CutPrefix(e.Text, "pubnote "); found {
			pubnote = note
		} else if note, found := strings.CutPrefix(e.Text, "privnote "); found {
			privnote = note
		}
	}
	http.Error(w, pubnote+"\n"+privnote, http.StatusOK)
}

func (db *DB) login(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		http.Error(w, "userapi.BadContentType", http.StatusBadRequest)
		return
	}
	username, password := req.FormValue("username"), req.FormValue("password")
	if username == "" || password == "" {
		http.Error(w, "userapi.LoginUsernameOrPasswordMissing", http.StatusBadRequest)
		return
	}
	if len(username) > 10 {
		http.Error(w, fmt.Sprintf("userapi.UsernameTooLong username=%q (must be at most 10 chars)", username), http.StatusBadRequest)
		return
	}
	for _, ch := range username {
		if ch < 'a' || 'z' < ch {
			http.Error(w, fmt.Sprintf("userapi.InvalidUsernameCharacter username=%q (must be [a-z]+)", username), http.StatusBadRequest)
			return
		}
	}
	uid, err := abname.New(username)
	if err != nil {
		http.Error(w, "userapi.EncodeLoginUsername: "+err.Error(), http.StatusBadRequest)
		return
	}

	db.mu.Lock()
	defer db.mu.Unlock()
	var pubnote, privnote, pwhash, pwsalt string
	for _, e := range alogdb.DefaultDB.Get("userapi." + username) {
		if note, found := strings.CutPrefix(e.Text, "pubnote "); found {
			pubnote = note
		} else if note, found := strings.CutPrefix(e.Text, "privnote "); found {
			privnote = note
		} else if rest, found := strings.CutPrefix(e.Text, "pwhash "); found {
			pwsalt, pwhash, _ = strings.Cut(rest, " ")
		}
	}
	if pwhash == "" {
		http.Error(w, "userapi.LoginUsernameNotFound", http.StatusBadRequest)
		return
	}
	if hash(username, password, pwsalt) != pwhash {
		log.Printf("userapi.BadPasswordAttempt username=%s", username)
		http.Error(w, "userapi.BadPassword", http.StatusBadRequest)
		return
	}

	var sid uint64
	sidany, found := db.userSessions.Load(uid)
	if !found {
		sid = rand64()
		db.userSessions.Store(uid, sid)
	} else {
		sid = sidany.(uint64)
	}
	sids := strconv.FormatUint(sid, 16)
	if !found {
		// Ignore login error: at worst the user needs to relogin later.
		alogdb.DefaultDB.Add("usersessions", username+" "+sids)
	}
	tohash := username + " " + salt + " " + sids
	hash := sha256.Sum256([]byte(tohash))
	sig := hex.EncodeToString(hash[:])
	w.Header().Add("Set-Cookie", fmt.Sprintf("session=%s.%s.%s; Max-Age=2147483647; SameSite=Strict", username, sig, sids))
	http.Error(w, pubnote+"\n"+privnote, http.StatusOK)
	log.Printf("userapi.UserLoggedIn username=%s", username)
}

func (db *DB) logout(w http.ResponseWriter, req *http.Request) {
	username := db.Username(w, req)
	if username == "" {
		http.Error(w, "userapi.NotLoggedIn", http.StatusPreconditionRequired)
		return
	}
	uid, err := abname.New(username)
	if err != nil {
		log.Printf("userapi.LogoutNewAbname username=%q: %v", username, err)
		http.Error(w, "userapi.LogoutNewAbname: "+err.Error(), http.StatusPreconditionRequired)
		return
	}
	log.Printf("userapi.UserLoggedOut username=%s", username)
	w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
	db.mu.Lock()
	defer db.mu.Unlock()
	db.userSessions.Delete(uid)
	if _, err := alogdb.DefaultDB.Add("usersessions", username+" 0"); err != nil {
		http.Error(w, "userapi.PersistLogout: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Error(w, "ok", http.StatusOK)
}

func (db *DB) update(w http.ResponseWriter, req *http.Request) {
	username := db.Username(w, req)
	if username == "" {
		http.Error(w, "userapi.NotLoggedIn", http.StatusPreconditionRequired)
		return
	}
	dbname := "userapi." + username
	pubnote, privnote, oldpassword, newpassword := req.FormValue("pubnote"), req.FormValue("privnote"), req.FormValue("oldpassword"), req.FormValue("newpassword")
	if strings.IndexByte(pubnote, 0) != -1 || strings.IndexByte(privnote, 0) != -1 {
		http.Error(w, "userapi.ZeroByteInNoteUpdate", http.StatusBadRequest)
		return
	}
	if strings.IndexByte(pubnote, '\n') != -1 || strings.IndexByte(privnote, '\n') != -1 {
		http.Error(w, "userapi.NewlineInNoteUpdate", http.StatusBadRequest)
		return
	}
	if len(pubnote) > 200 || len(privnote) > 200 {
		http.Error(w, "userapi.NoteTooLongInNoteUpdate", http.StatusBadRequest)
		return
	}

	if (req.Form.Has("oldpassword") || req.Form.Has("newpassword")) && (oldpassword == "" || newpassword == "") {
		http.Error(w, "userapi.NewOrOldPasswordMissing", http.StatusBadRequest)
		return
	}

	db.mu.Lock()
	db.mu.Unlock()
	var oldpubnote, oldprivnote, oldpwsalt, oldpwhash string
	for _, e := range alogdb.DefaultDB.Get(dbname) {
		if note, found := strings.CutPrefix(e.Text, "pubnote "); found {
			oldpubnote = note
		} else if note, found := strings.CutPrefix(e.Text, "privnote "); found {
			oldprivnote = note
		} else if rest, found := strings.CutPrefix(e.Text, "pwhash "); found {
			oldpwsalt, oldpwhash, _ = strings.Cut(rest, " ")
		}
	}

	var items []string
	if req.Form.Has("pubnote") && pubnote != oldpubnote {
		items = append(items, "pubnote "+pubnote)
	}
	if req.Form.Has("privnote") && privnote != oldprivnote {
		items = append(items, "privnote "+privnote)
	}
	if newpassword != "" {
		if hash(username, oldpassword, oldpwsalt) != oldpwhash {
			log.Printf("userapi.BadOldPassword username=%s", username)
			http.Error(w, "userapi.BadOldPassword", http.StatusBadRequest)
			return
		}
		if newpassword != oldpassword {
			newpwsalt := randsalt()
			items = append(items, "pwhash "+newpwsalt+" "+hash(username, newpassword, newpwsalt))
		}
	}
	if _, err := alogdb.DefaultDB.Add(dbname, items...); err != nil {
		http.Error(w, "userapi.UpdateData: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("userapi.UpdatedUser user=%s", username)
	msgz.Default.Printf("userapi.UpdatedUser username=%s", username)
	http.Error(w, "ok", http.StatusOK)
}

func (db *DB) Username(w http.ResponseWriter, req *http.Request) string {
	sessioncookie, err := req.Cookie("session")
	if err != nil || sessioncookie.Value == "" {
		return ""
	}
	parts := strings.Split(sessioncookie.Value, ".")
	if len(parts) <= 1 {
		w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
		return ""
	}
	user, sig := parts[0], parts[1]
	tohash := user + " " + salt
	if len(parts) == 3 {
		tohash += " " + parts[2]
	}
	hash := sha256.Sum256([]byte(tohash))
	wantsig := hex.EncodeToString(hash[:])
	if sig != wantsig {
		log.Printf("userapi.LogoutDueBadSignature username=%s", user)
		w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
		return ""
	}
	if strings.HasSuffix(user, "-guest") {
		return user
	}

	// Handle registered users.
	if len(parts) != 3 {
		w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
		return ""
	}
	uid, _ := abname.New(user)
	wantsidany, found := db.userSessions.Load(uid)
	if !found {
		log.Printf("userapi.LogoutDueToDeletedSession username=%s", user)
		w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
		return ""
	}
	wantsid := wantsidany.(uint64)
	if sid, _ := strconv.ParseUint(parts[2], 16, 64); sid != wantsid {
		log.Printf("userapi.LogoutDueToBadSessionID username=%s", user)
		w.Header().Add("Set-Cookie", "session=; Max-Age=-1; SameSite=Strict")
		return ""
	}
	return user
}

func (db *DB) printUser(w http.ResponseWriter, req *http.Request) {
	if user := db.Username(w, req); user != "" {
		fmt.Fprintf(w, "%q\n", user)
		return
	}
	http.Error(w, "userapi.NotLoggedIn", http.StatusUnauthorized)
}

func tenure(regdate, now time.Time) string {
	regMonth := regdate.Year()*12 + int(regdate.Month()-time.January)
	nowMonth := now.Year()*12 + int(now.Month()-time.January)
	months := nowMonth - regMonth
	if months == 0 {
		return fmt.Sprintf("%04d-%s (this month)", regdate.Year(), regdate.Month())
	} else if months == 1 {
		return fmt.Sprintf("%04d-%s (last month)", regdate.Year(), regdate.Month())
	} else if months < 12 {
		return fmt.Sprintf("%04d-%s (%d months ago)", regdate.Year(), regdate.Month(), months)
	} else if months < 24 {
		return fmt.Sprintf("%04d-%s (1 year ago)", regdate.Year(), regdate.Month())
	} else {
		return fmt.Sprintf("%04d-%s (%d years ago)", regdate.Year(), regdate.Month(), months/12)
	}
}

// Userinfo returns public information about the user.
// It has the format of "YYYY-MM (x years)\npublic note if any".
func (db *DB) Userinfo(username string, now time.Time) string {
	if guest, ok := strings.CutSuffix(username, "-guest"); ok {
		if len(guest) < guestIDLen+2 {
			return "userapi.BadGuestName"
		}
		monthStampString := guest[:len(guest)-guestIDLen]
		monthStamp := int(monthStampString[0]-'a')*26 + int(monthStampString[1]-'a')
		if len(monthStampString) == 3 {
			monthStamp = monthStamp*26 + int(monthStampString[2]-'a')
		}
		return tenure(time.Date(2025+monthStamp/12, time.Month(monthStamp%12)+time.January, 1, 0, 0, 0, 0, time.UTC), now)
	}
	if strings.IndexByte(username, '-') != -1 {
		return ""
	}

	entries := alogdb.DefaultDB.Get("userapi." + username)
	if len(entries) == 0 {
		return "alogdb.UserinfoForDeletedUser"
	}
	tenure := tenure(time.UnixMilli(entries[0].TS), now)
	var pubnote string
	for _, e := range alogdb.DefaultDB.Get("userapi." + username) {
		if note, found := strings.CutPrefix(e.Text, "pubnote "); found {
			pubnote = note
		}
	}
	return tenure + "\n" + html.EscapeString(pubnote)
}
