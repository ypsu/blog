// markdump is an effdump of the rendered posts.
// run `go run blog/markdump` to use it.
package main

import (
	"blog/alogdb"
	"blog/markup"
	"blog/posts"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"regexp"

	"github.com/ypsu/effdump"
	"github.com/ypsu/textar"
)

type nopIO struct{}

func (nopIO) Read([]byte) (int, error)  { return 0, io.EOF }
func (nopIO) Write([]byte) (int, error) { return 0, nil }

func run() error {
	markupContent, err := os.ReadFile("markup/testdata.textar")
	if err != nil {
		return fmt.Errorf("markdump.LoadMarkupTestdata: %v", err)
	}
	markupTests := textar.Parse(markupContent)
	if len(markupTests.Files) < 5 {
		return fmt.Errorf("markdump.TestdataMarkupMissing len=%d", len(markupTests.Files))
	}

	dump := effdump.New("markdump")

	for name, data := range markupTests.Range() {
		dump.Add("markup/"+name, markup.Render(string(data), false))
		dump.Add("markup/"+name+".restricted", markup.Render(string(data), true))
	}

	if _, err := os.Stat("docs"); errors.Is(err, fs.ErrNotExist) {
		os.Chdir("..")
	}
	if _, err := os.Stat("docs"); err != nil {
		return fmt.Errorf("markdump.StatDocs: %v", err)
	}

	db, err := alogdb.NewForTesting(nopIO{})
	if err != nil {
		return fmt.Errorf("markdump.NewAlogdb: %v", err)
	}
	alogdb.DefaultDB = db

	log.SetOutput(io.Discard)
	posts.LoadPosts()
	re := regexp.MustCompile("\"PostRenderTS\": [0-9]+")
	for k, v := range posts.Dump() {
		dump.Add("posts/"+k, re.ReplaceAllString(v, "\"PostRenderTS\": 1  // markdump.PlaceholderValue"))
	}
	for k, v := range posts.DumpRSS() {
		dump.Add("rss/"+k, v)
	}

	dump.Run(context.Background())
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
