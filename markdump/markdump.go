// markdump is an effdump of the rendered posts.
// run `go run blog/markdump` to use it.
package main

import (
	"blog/alogdb"
	"blog/markdown"
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
	db, err := alogdb.NewForTesting(nopIO{})
	if err != nil {
		return fmt.Errorf("markdump.NewAlogdb: %v", err)
	}
	alogdb.DefaultDB = db

	log.SetOutput(io.Discard)
	if _, err := os.Stat("docs"); errors.Is(err, fs.ErrNotExist) {
		os.Chdir("..")
	}
	if _, err := os.Stat("docs"); err != nil {
		return fmt.Errorf("markdump.StatDocs: %v", err)
	}
	testdataContent, err := os.ReadFile("markdown/testdata.textar")
	if err != nil {
		return fmt.Errorf("markdump.LoadTestdata: %v", err)
	}
	testdata := textar.Parse(testdataContent)
	if len(testdata.Files) < 5 {
		return fmt.Errorf("markdump.TestdataMissing len=%d", len(testdata.Files))
	}

	dump := effdump.New("markdump")

	for name, data := range testdata.Range() {
		dump.Add("tests/"+name, markdown.Render(string(data), false))
		dump.Add("tests/"+name+".restricted", markdown.Render(string(data), true))
	}

	posts.LoadPosts()
	re := regexp.MustCompile("let PostRenderTS = [0-9]+")
	for k, v := range posts.Dump() {
		dump.Add("posts/"+k, re.ReplaceAllString(v, "let PostRenderTS = 1  // markdump.PlaceholderValue"))
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
