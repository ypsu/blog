package testwriter

import (
	"os"
	"testing"
)

func readOutput(t *testing.T) string {
	f, err := os.ReadFile("tmpdata")
	if err != nil {
		t.Fatal(err)
	}
	return string(f)
}

func TestData(t *testing.T) {
	in, out := Data(t)
	oldwrite := *writeFlag
	*writeFlag = true
	*fileFlag = "tmpdata"
	defer func() {
		*writeFlag = oldwrite
		*fileFlag = "test.data"
		if err := os.Remove("tmpdata"); err != nil {
			t.Fatal(err)
		}
	}()

	t.Run("pass", func(t *testing.T) {
		os.WriteFile("tmpdata", []byte(in["pass"]), 0666)
		tin, tout := Data(t)
		tin["testentry"] = "some input"
		tout["testentry"] = "some output"
	})
	out["pass"] = readOutput(t)

	t.Run("mismatch", func(t *testing.T) {
		os.WriteFile("tmpdata", []byte(in["mismatch"]), 0666)
		tin, tout := Data(t)
		tin["testentry"] = "some input"
		tout["testentry"] = "some output"
	})
	out["mismatch"] = readOutput(t)

	t.Run("add", func(t *testing.T) {
		os.WriteFile("tmpdata", []byte(in["add"]), 0666)
		_, tout := Data(t)
		tout["2"] = "222"
	})
	out["add"] = readOutput(t)

	t.Run("del", func(t *testing.T) {
		os.WriteFile("tmpdata", []byte(in["del"]), 0666)
		_, tout := Data(t)
		delete(tout, "2")
	})
	out["del"] = readOutput(t)
}
