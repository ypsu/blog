// markdump is an effdump of the rendered posts.
// run `go run blog/markdump` to use it.
package main

import (
	"blog/posts"
	"context"
	"io"
	"log"
)

func main() {
	log.SetOutput(io.Discard)
	posts.LoadPosts()
	posts.Dump().Run(context.Background())
}
