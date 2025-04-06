// markdump is an effdump of the rendered posts.
// run `go run blog/markdump` to use it.
package main

import (
	"blog/markdown"
	"blog/posts"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/ypsu/effdump"
	"github.com/ypsu/textar"
)

func run() error {
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
	if len(testdata) < 5 {
		return fmt.Errorf("markdump.TestdataMissing len=%d", len(testdata))
	}

	dump := effdump.New("markdump")

	for _, f := range testdata {
		dump.Add("tests/"+f.Name, markdown.Render(string(f.Data), false))
		dump.Add("tests/"+f.Name+".restricted", markdown.Render(string(f.Data), true))
	}

	posts.LoadPosts()
	for k, v := range posts.Dump() {
		dump.Add("posts/"+k, v)
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
