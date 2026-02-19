package alogdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ypsu/efftesting/efft"
)

func TestAlogdb(t *testing.T) {
	efft.Init(t)
	Now = func() int64 { return 1e6 }
	logfile := filepath.Join(t.TempDir(), "log")
	fh := efft.Must1(os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0644))
	db := efft.Must1(NewForTesting(fh))

	add := func(name string, texts ...string) any {
		t.Helper()
		ts, err := db.Add(name, texts...)
		if err != nil {
			return strings.ReplaceAll(err.Error(), logfile, "[logfile]")
		}
		return ts
	}

	efft.Effect(add("testlog", "hello world")).Equals("1000000")
	efft.Effect(add("testlog", "hello world 2")).Equals("1000001")
	efft.Effect(add("testlog2", "hello world 3")).Equals("1000002")
	efft.Effect(add("testlog", "hello world 4", "hello world 5", "hello world 6")).Equals("1000003")
	efft.Effect(add("", "hello world")).Equals("alogdb.InvalidName name=")
	efft.Effect(add("test app", "hello world")).Equals("alogdb.InvalidName name=test app")
	efft.Effect(add("testlog", "hello\000world")).Equals("alogdb.ZeroByteText")
	fh.Close()
	efft.Effect(add("testlog", "hello world")).Equals("alogdb.WriteFile: write [logfile]: file already closed")

	filedata := efft.Must1(os.ReadFile(logfile))
	efft.Effect(strings.ReplaceAll(string(filedata), "\000", "")).Equals(`
		1000000 testlog hello world
		1000001 testlog hello world 2
		1000002 testlog2 hello world 3
		1000003 testlog hello world 4
		1000004 testlog hello world 5
		1000005 testlog hello world 6
	`)

	efft.Effect(db.Get("testlog")).Equals(`
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
	efft.Effect(db.Get("testlog2")).Equals(`
		[
		  {
		    "TS": 1000002,
		    "Text": "hello world 3"
		  }
		]`,
	)
}
