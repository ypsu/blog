package markdown

import (
	"blog/testwriter"
	"testing"
)

func TestRender(t *testing.T) {
	in, out := testwriter.Data(t)
	for k, v := range in {
		out[k] = Render(v, false)
		out[k+"_restricted"] = Render(v, true)
	}
}
