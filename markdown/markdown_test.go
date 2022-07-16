package markdown

import (
	"notech/testwriter"
	"testing"
)

func TestRender(t *testing.T) {
	in, out := testwriter.Data(t)
	for k, v := range in {
		out[k] = Render(v)
		out[k+"_restricted"] = Render(v)
	}
}
