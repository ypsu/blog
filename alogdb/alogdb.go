// Package alogdb implements a cloudflare D1 backed actionlog database.
// The SQL database is just a list of (unix_timestamp_ms, name, logentry) rows.
//
// There's no support for deletion through this package.
// DB cleanups must be done externally.
// This package won't notice deleted rows until the next restart.
// Delete only the rows that the specified appid doesn't care about anyway.
//
// See https://developers.cloudflare.com/api/operations/cloudflare-d1-query-database for cloudflare's documentation.
package alogdb

import (
	"blog/msgz"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var DefaultDB *DB

type DB struct {
	// Lock protected dynamic data below.
	// These locks protect everything.
	// Usage should be low enough that this is fine.
	mu, wmu         sync.Mutex
	lastTS          int64
	alogs           map[string]*strings.Builder
	writes          int // number of writes in the current period
	lastwritePeriod int64
	logfile         *os.File // db backed with this file instead of clodflare if non-nil, testing only
}

// Now is time.Now() but overridable in tests.
var Now = func() int64 { return time.Now().UnixMilli() }

func New(ctx context.Context) (*DB, error) {
	db := &DB{alogs: map[string]*strings.Builder{}}
	buf, err := callAPI("GET", "/api/alogdb", "")
	if err != nil {
		return nil, fmt.Errorf("alogdb.LoadData: %v", err)
	}
	if err := db.addBatch(string(buf)); err != nil {
		return nil, fmt.Errorf("alogdb.AddData: %v", err)
	}
	return db, nil
}

func NewForTesting(f *os.File) (*DB, error) {
	db := &DB{
		alogs:   map[string]*strings.Builder{},
		logfile: f,
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("alogdb.ReadFile: %v", err)
	}
	if err := db.addBatch(string(buf)); err != nil {
		return nil, fmt.Errorf("alogdb.AddTestdata: %v", err)
	}
	return db, nil
}

func (db *DB) addBatch(buf string) error {
	for logentry := range strings.SplitSeq(buf, "\000\n") {
		if logentry == "" {
			continue
		}
		parts := strings.SplitN(logentry, " ", 3)
		if len(parts) != 3 {
			return fmt.Errorf("alogdb.InvalidFile logentry=%q", logentry)
		}
		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return fmt.Errorf("alogdb.InvalidTSInFile logentry=%q: %v", logentry, err)
		}
		db.addToMap(ts, parts[1], parts[2])
	}
	return nil
}

// Assumes that db.mu is locked or the lock is not needed (i.e. during init).
func (db *DB) addToMap(ts int64, name, text string) {
	alog, ok := db.alogs[name]
	if !ok {
		alog = &strings.Builder{}
		db.alogs[name] = alog
	}
	fmt.Fprintf(alog, "%d\000%s\000", ts, text)
	db.lastTS = ts
}

var namere = regexp.MustCompile("^[a-zA-Z0-9.]+$")

// Add writes the list of (text1, text2, text3, ...) to the name DB.
// Either all rows were written or none.
// The return value is the timestamp of the first row, rest is +1ms, +2ms, and so on.
// Note: there's a small chance that the write makes to the DB yet this function returns an error.
// The write will appear only after the next server restart.
// The clients must handle such spurious entries correctly.
// Generally the clients should handle any garbage data gracefully anyway.
func (db *DB) Add(name string, texts ...string) (int64, error) {
	if !namere.MatchString(name) {
		return 0, fmt.Errorf("alogdb.InvalidName name=%s", name)
	}
	for _, text := range texts {
		if strings.IndexByte(text, 0) != -1 {
			return 0, fmt.Errorf("alogdb.ZeroByteText")
		}
	}

	db.wmu.Lock()
	defer db.wmu.Unlock()
	ts := Now()
	db.mu.Lock()
	if ts <= db.lastTS {
		ts = db.lastTS + 1
	}
	const maxwrites, periodLength = 100, 300_000 // limit: 100 writes / 5 minutes
	currentPeriod := ts / periodLength
	if db.lastwritePeriod == currentPeriod {
		if db.writes == maxwrites {
			return 0, fmt.Errorf("alogdb.RatelimitReached limit=%dw/%ds", maxwrites, periodLength/1000)
		}
		db.writes++
	} else {
		db.writes, db.lastwritePeriod = 1, currentPeriod
	}
	db.mu.Unlock()

	if db.logfile != nil {
		for i, text := range texts {
			if _, err := fmt.Fprintf(db.logfile, "%d %s %s\000\n", ts+int64(i), name, text); err != nil {
				return 0, fmt.Errorf("alogdb.WriteFile: %v", err)
			}
		}
	} else {
		if _, err := callAPI("POST", fmt.Sprintf("/api/alogdb?name=%s&ts=%d", name, ts), strings.Join(texts, "\000")); err != nil {
			msgz.Default.Printf("alogdb.Append name=%s content=%q: %v", name, strings.Join(texts, "|||"), err)
			errmsg := fmt.Errorf("alogdb.Append name=%s: %v", name, err)
			log.Print(errmsg)
			return 0, errmsg
		}
	}

	db.mu.Lock()
	for i, text := range texts {
		db.addToMap(ts+int64(i), name, text)
	}
	db.mu.Unlock()
	return ts, nil
}

type Entry struct {
	TS   int64
	Text string
}

func (db *DB) Get(name string) []Entry {
	db.mu.Lock()
	alog := db.alogs[name]
	if alog == nil {
		db.mu.Unlock()
		return nil
	}
	s := alog.String()
	db.mu.Unlock()

	parts := strings.Split(s, "\000")
	entries := make([]Entry, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		ts, err := strconv.ParseInt(parts[i], 10, 64)
		if err != nil {
			log.Printf("alogdb.InvalidEntryInGet name=%s i=%d ts=%q: %v", name, i/2, parts[i], err)
			continue
		}
		entries[i/2] = Entry{ts, parts[i+1]}
	}
	return entries
}

var apiAddress = flag.String("apiaddr", "http://localhost:8787", "The address of cloudflare api.")
var apikey = os.Getenv("APIKEY") // api.iio.ie key

// callAPI invokes the specific api method over http.
// the response's body is returned iff error is nil.
func callAPI(method, url, body string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, *apiAddress+url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("alogdb.NewRequestWithContext: %v", err)
	}
	req.Header.Add("apikey", apikey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alogdb.Do: %v", err)
	}
	rbody, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("alogdb.ReadBody: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		log.Printf("alogdb.UncleanRequestClose: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alogdb.StatusCheck url=%s method=%s status=%q body=%q", url, method, resp.Status, bytes.TrimSpace(rbody))
	}
	return rbody, nil
}
