package alogdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ypsu/efftesting"
)

func assertNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
}

func TestAlogdb(t *testing.T) {
	et := efftesting.New(t)
	Now = func() int64 { return 1e6 }
	logfile := filepath.Join(t.TempDir(), "log")
	fh, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0644)
	assertNil(t, err)
	db, err := NewForTesting(fh)
	assertNil(t, err)

	add := func(name string, texts ...string) any {
		t.Helper()
		ts, err := db.Add(name, texts...)
		if err != nil {
			return strings.ReplaceAll(err.Error(), logfile, "[logfile]")
		}
		return ts
	}

	et.Expect("HelloWorld1", add("testlog", "hello world"), "1000000")
	et.Expect("HelloWorld2", add("testlog", "hello world 2"), "1000001")
	et.Expect("HelloWorld3", add("testlog2", "hello world 3"), "1000002")
	et.Expect("HelloWorldMulti", add("testlog", "hello world 4", "hello world 5", "hello world 6"), "1000003")
	et.Expect("BadName1", add("", "hello world"), "alogdb.InvalidName name=")
	et.Expect("BadName2", add("test app", "hello world"), "alogdb.InvalidName name=test app")
	et.Expect("BadContent", add("testlog", "hello\000world"), "alogdb.ZeroByteText")
	fh.Close()
	et.Expect("BackendError", add("testlog", "hello world"), "alogdb.WriteFile: write [logfile]: file already closed")

	filedata, err := os.ReadFile(logfile)
	assertNil(t, err)
	et.Expect("BackendContent", strings.ReplaceAll(string(filedata), "\000", ""), `
		1000000 testlog hello world
		1000001 testlog hello world 2
		1000002 testlog2 hello world 3
		1000003 testlog hello world 4
		1000004 testlog hello world 5
		1000005 testlog hello world 6
	`)

	et.Expect("Get1", db.Get("testlog"), `
		[
		  {
		    "TS": 1000000,
		    "Text": "hello world"
		  },
		  {
		    "TS": 1000001,
		    "Text": "hello world 2"
		  },
		  {
		    "TS": 1000003,
		    "Text": "hello world 4"
		  },
		  {
		    "TS": 1000004,
		    "Text": "hello world 5"
		  },
		  {
		    "TS": 1000005,
		    "Text": "hello world 6"
		  }
		]`)
	et.Expect("Get2", db.Get("testlog2"), `
		[
		  {
		    "TS": 1000002,
		    "Text": "hello world 3"
		  }
		]`,
	)
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
