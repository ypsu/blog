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
	for name, data := range textar.Parse(testdataContent).Range() {
		dump.Add(name, Render(string(data), false))
		dump.Add(name+".restricted", Render(string(data), true))
	}
	et := efftesting.New(t)
	et.Expect("TestHash", fmt.Sprintf("0x%016x", dump.Hash()), "0xd5aaaaa48728718e")
	t.Log("Use `go run blog/markdump` to examine the diff.")
}

func TestMain(m *testing.M) {
	os.Exit(efftesting.Main(m))
}
