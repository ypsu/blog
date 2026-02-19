package markdown

import (
	"fmt"
	"testing"

	_ "embed"

	"github.com/ypsu/effdump"
	"github.com/ypsu/efftesting/efft"
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
	efft.Init(t)
	efft.Effect(fmt.Sprintf("0x%016x", dump.Hash())).Equals("0xd5aaaaa48728718e")
	t.Log("Use `go run blog/markdump` to examine the diff.")
}
