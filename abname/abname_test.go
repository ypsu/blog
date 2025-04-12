package abname_test

import (
	"blog/abname"
	"os"
	"strconv"
	"testing"

	"github.com/ypsu/efftesting"
)

func init() {
	if err := abname.Init(); err != nil {
		panic(err)
	}
}

func TestAbnames(t *testing.T) {
	et := efftesting.New(t)

	newID := func(s string) string {
		v, err := abname.New(s)
		if err != nil {
			return err.Error()
		}
		return strconv.FormatInt(int64(v), 10)
	}

	et.Expect("", newID("alice"), "43833630720")
	et.Expect("", newID("frobber"), "159721911271424")
	et.Expect("", newID("frobber-admins"), "159721911271436")
	et.Expect("", newID("frobber-team"), "159721911271433")
	et.Expect("", newID("frobber-9"), "159721911271433")
	et.Expect("", newID("frobber-9000"), "abname.ErrBadForenameID")

	et.Expect("", abname.ID(43833630720).String(), "alice")
	et.Expect("", abname.ID(159721911271424).String(), "frobber")
	et.Expect("", abname.ID(159721911271433).String(), "frobber-team")
	et.Expect("", abname.ID(159721911272424).String(), "frobber-1000")
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
