package abname_test

import (
	"blog/abname"
	"strconv"
	"testing"

	"github.com/ypsu/efftesting/efft"
)

func init() {
	if err := abname.Init(); err != nil {
		panic(err)
	}
}

func TestAbnames(t *testing.T) {
	efft.Init(t)

	newID := func(s string) string {
		v, err := abname.New(s)
		if err != nil {
			return err.Error()
		}
		return strconv.FormatInt(int64(v), 10)
	}

	efft.Effect(newID("alice")).Equals("43833630720")
	efft.Effect(newID("frobber")).Equals("159721911271424")
	efft.Effect(newID("frobber-admins")).Equals("159721911271436")
	efft.Effect(newID("frobber-team")).Equals("159721911271433")
	efft.Effect(newID("frobber-9")).Equals("159721911271433")
	efft.Effect(newID("frobber-9000")).Equals("abname.ErrBadForenameID")

	efft.Effect(abname.ID(43833630720).String()).Equals("alice")
	efft.Effect(abname.ID(159721911271424).String()).Equals("frobber")
	efft.Effect(abname.ID(159721911271433).String()).Equals("frobber-team")
	efft.Effect(abname.ID(159721911272424).String()).Equals("frobber-1000")
}
