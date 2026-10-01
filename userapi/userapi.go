// Package userapi manages user registrations and logins.
package userapi

import (
	"blog/abname"
	"blog/alogdb"
	"blog/eventz"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	pseudorand "math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const GuestIDLen = 5 // this is the random part, the timestamp is not included

var salt = os.Getenv("SALT")

type DB struct {
	mu               sync.Mutex
	lastreg          time.Time
	lastLoginAttempt time.Time

	userSessions sync.Map // should be map[abname.ID]uint64
}

// Overrideable for testing.
var now = func() time.Time { return time.Now() }

func (db *DB) Init() {
	if salt == "" {
		log.Printf("userapi.NoSalt")
		eventz.Default.Printf("userapi.NoSalt")
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

func (db *DB) HandleHTTP(w http.ResponseWriter, req *http.Request, username string, secure bool) {
	if req.Method != "POST" {
		http.Error(w, fmt.Sprintf("userapi.InvalidMethod method=%s (must be POST)", req.Method), http.StatusMethodNotAllowed)
		return
	}
	if req.ContentLength == -1 || req.ContentLength > 1e5 {
		http.Error(w, "userapi.BadContentLength len="+strconv.FormatInt(req.ContentLength, 10), http.StatusBadRequest)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(w, "userapi.ParseForm: "+err.Error(), http.StatusBadRequest)
		return
	}

	action := req.FormValue("action")
	switch action {
	case "username":
		db.printUser(w, req, username)
	case "login":
		db.login(w, req, secure)
	case "logout":
		db.logout(w, req, username, secure)
	case "registerguest":
		db.registerGuest(w, req, username, secure)
	case "register":
		db.registerFull(w, req, secure)
	case "update":
		db.update(w, req, username)
	case "userdata":
		db.userdata(w, req, username)
	case "":
		http.Error(w, "userapi.EmptyAction (missing POST body?)", http.StatusBadRequest)
	default:
		http.Error(w, "userapi.InvalidAction action="+action, http.StatusBadRequest)
	}
}

func (db *DB) registerGuest(w http.ResponseWriter, req *http.Request, user string, secure bool) {
	if user != "" {
		http.Error(w, user, http.StatusAlreadyReported)
		return
	}

	now, userid := now(), make([]byte, 0, 10)
	year, month := now.Year()%100, int(now.Month()-time.January)+1
	userid = append(userid, byte('a'+year/10), byte('a'+year%10), byte('a'+month))
	for range GuestIDLen {
		userid = append(userid, byte('a'+pseudorand.IntN(26)))
	}
	username := string(userid) + "-guest"
	tohash := username + " " + salt
	hash := sha256.Sum256([]byte(tohash))
	sig := hex.EncodeToString(hash[:])

	log.Printf("userapi.RegisteredGuest user=%s", username)
	eventz.Default.Printf("userapi.RegisteredGuest user=%s", username)
	SetSessionCookies(w, username, username+"."+sig, secure)
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

func (db *DB) registerFull(w http.ResponseWriter, req *http.Request, secure bool) {
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
	eventz.Default.Printf("userapi.RegisterAttempt username=%q", username)

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
		eventz.Default.Printf("userapi.TooManyRegistrations username=%s", username)
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
	SetSessionCookies(w, username, username+"."+sig+"."+sids, secure)
	http.Error(w, "ok", http.StatusOK)
	log.Printf("userapi.UserRegistered username=%s", username)
	eventz.Default.Printf("userapi.RegisteredUser username=%s", username)
}

func (db *DB) userdata(w http.ResponseWriter, req *http.Request, username string) {
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

func (db *DB) login(w http.ResponseWriter, req *http.Request, secure bool) {
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

	if time.Since(db.lastLoginAttempt) < 2*time.Second {
		eventz.Default.Printf("userapi.TooManyLoginAttempts username=%s", username)
		http.Error(w, "userapi.TooManyLoginAttempts (try 2 seconds later)", http.StatusTooManyRequests)
		return
	}
	db.lastLoginAttempt = time.Now()

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
	SetSessionCookies(w, username, username+"."+sig+"."+sids, secure)
	http.Error(w, pubnote+"\n"+privnote, http.StatusOK)
	log.Printf("userapi.UserLoggedIn username=%s", username)
}

func (db *DB) logout(w http.ResponseWriter, req *http.Request, username string, secure bool) {
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
	SetSessionCookies(w, "", "", secure)
	db.mu.Lock()
	defer db.mu.Unlock()
	db.userSessions.Delete(uid)
	if _, err := alogdb.DefaultDB.Add("usersessions", username+" 0"); err != nil {
		http.Error(w, "userapi.PersistLogout: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Error(w, "ok", http.StatusOK)
}

func (db *DB) update(w http.ResponseWriter, req *http.Request, username string) {
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
	defer db.mu.Unlock()
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
	if len(items) > 0 {
		if _, err := alogdb.DefaultDB.Add(dbname, items...); err != nil {
			http.Error(w, "userapi.UpdateData: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("userapi.UpdatedUser user=%s", username)
		eventz.Default.Printf("userapi.UpdatedUser username=%s", username)
	}
	http.Error(w, "ok", http.StatusOK)
}

func (db *DB) Session(req *http.Request) (username, session string, hadSession bool) {
	sessioncookie, err := req.Cookie("session")
	if err != nil || sessioncookie.Value == "" {
		_, err := req.Cookie("username")
		return "", "", err == nil // clear the leftover username too
	}
	parts := strings.Split(sessioncookie.Value, ".")
	if len(parts) <= 1 {
		log.Printf("userapi.LogoutDueBadSessionCookie")
		return "", "", true
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
		return "", "", true
	}
	if strings.HasSuffix(user, "-guest") {
		return user, sessioncookie.Value, true
	}

	// Handle registered users.
	if len(parts) != 3 {
		log.Printf("userapi.LogoutDueBadRegisteredSessionCookie user=%s", user)
		return "", "", true
	}
	uid, _ := abname.New(user)
	wantsidany, found := db.userSessions.Load(uid)
	if !found {
		log.Printf("userapi.LogoutDueToDeletedSession username=%s", user)
		return "", "", true
	}
	wantsid := wantsidany.(uint64)
	if sid, _ := strconv.ParseUint(parts[2], 16, 64); sid != wantsid {
		log.Printf("userapi.LogoutDueToBadSessionID username=%s", user)
		return "", "", true
	}
	return user, sessioncookie.Value, true
}

func (db *DB) printUser(w http.ResponseWriter, req *http.Request, user string) {
	if user == "" {
		http.Error(w, "userapi.NotLoggedIn", http.StatusUnauthorized)
		return
	}
	fmt.Fprintf(w, "%q\n", user)
}

// SetSessionCookies sets or, if session is empty, clears the session cookies.
func SetSessionCookies(w http.ResponseWriter, username, session string, secure bool) {
	maxAge := math.MaxInt32
	if session == "" {
		maxAge, username = -1, ""
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: session, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: "username", Value: username, Path: "/", MaxAge: maxAge, Secure: secure, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
}
