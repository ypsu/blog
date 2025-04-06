// markdump is an effdump of the rendered posts.
// run `go run blog/markdump` to use it.
package main

import (
	"blog/posts"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/ypsu/effdump"
)

func run() error {
	log.SetOutput(io.Discard)
	if _, err := os.Stat("docs"); errors.Is(err, fs.ErrNotExist) {
		os.Chdir("..")
	}
	if _, err := os.Stat("docs"); err != nil {
		return fmt.Errorf("markdump.DocsDirNotFound: %v", err)
	}

	dump := effdump.New("markdump")

	posts.LoadPosts()
	for k, v := range posts.Dump() {
		dump.Add("posts/"+k, v)
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
