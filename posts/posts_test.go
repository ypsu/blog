package posts

import (
	"blog/testwriter"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

type testResponse struct {
	hdr http.Header
	buf bytes.Buffer
}

func (r *testResponse) Header() http.Header           { return r.hdr }
func (*testResponse) WriteHeader(int)                 {}
func (r *testResponse) Write(buf []byte) (int, error) { return r.buf.Write(buf) }

func TestHandlers(t *testing.T) {
	_, outputs := testwriter.Data(t)
	for k := range outputs {
		delete(outputs, k)
	}
	LoadPosts()

	query := func(page string) {
		url, err := url.Parse(page)
		if err != nil {
			t.Fatal(err)
		}
		request := &http.Request{URL: url}
		response := &testResponse{hdr: http.Header{}}
		HandleHTTP(response, request)
		ctype := fmt.Sprintf("Content-Type: %s\n", response.hdr["Content-Type"])
		outputs[page] = ctype + response.buf.String()
	}

	dirents, err := os.ReadDir(".")
	if err != nil {
		log.Fatal(err)
	}
	for _, ent := range dirents {
		if !strings.HasPrefix(ent.Name(), "sample") {
			continue
		}
		query(ent.Name())
	}
	query("frontpage")
	query("latest")
	query("rss")
}
