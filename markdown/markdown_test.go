package markdown

import (
	"fmt"
	"os"
	"testing"

	_ "embed"

	"github.com/ypsu/effdump"
	"github.com/ypsu/efftesting"
	"github.com/ypsu/textar"
)

//go:embed testdata.textar
var testdataContent []byte

func TestRender(t *testing.T) {
	dump := effdump.New("markdown")
	for _, f := range textar.Parse(testdataContent) {
		dump.Add(f.Name, Render(string(f.Data), false))
		dump.Add(f.Name+".restricted", Render(string(f.Data), true))
	}
	et := efftesting.New(t)
	et.Expect("TestHash", fmt.Sprintf("0x%016x", dump.Hash()), "0xd5aaaaa48728718e")
	t.Log("Use `go run blog/markdump` to examine the diff.")
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
