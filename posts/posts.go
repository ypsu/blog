// package posts implements the http/gopher handlers for serving my posts.
package posts

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"notech/markdown"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var postPath = flag.String("postpath", ".", "path to the posts")
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var titleRE = regexp.MustCompile(`(?:^#|\n!title) (\w+):? ([^\n]*)`)

type post struct {
	name, subtitle, created string
	content, rawcontent     []byte
	contentType             string
	lastmod                 time.Time
}

var postsCache = &map[string]post{}

func htmlHeader(title string) string {
	return fmt.Sprintf(`<!doctype html><html lang=en><head>
  <title>%s</title>
  <meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'>
  <style>@media screen { body { max-width:50em;font-family:sans-serif } }</style>
</head><body>
`, title)
}

func loadPost(name string, cachedPost post) (post, bool) {
	newPost := post{name: name}

	// check for modification.
	fileinfo, err := os.Stat(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return post{}, false
	}
	newPost.lastmod = fileinfo.ModTime()
	if newPost.lastmod == cachedPost.lastmod {
		return cachedPost, true
	}

	// load the content.
	log.Printf("loading %s", name)
	newPost.content, err = os.ReadFile(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return post{}, false
	}
	newPost.rawcontent = newPost.content

	// extract title and subtitle.
	titles := titleRE.FindSubmatch(newPost.rawcontent)
	if len(titles) == 3 {
		if name != string(titles[1]) {
			log.Printf("wrong title in %s: %s", name, titles[1])
		}
		newPost.subtitle = string(titles[2])
	}

	// extract the creation date if available.
	created := createdRE.Find(newPost.content)
	if len(created) != 0 {
		newPost.created = string(created[10:])
	}

	// convert to html if it was a markdown file.
	if bytes.HasPrefix(newPost.content, []byte("# ")) {
		buf := &bytes.Buffer{}
		buf.WriteString(htmlHeader(newPost.name))
		buf.WriteString(markdown.Render(string(newPost.content)))
		buf.WriteString("<hr><p><a href=/>to the frontpage</a></p>\n")
		buf.WriteString("</body></html>\n")
		newPost.content = buf.Bytes()
	}

	newPost.contentType = http.DetectContentType(newPost.content)

	return newPost, true
}

func orderedEntries(posts map[string]post) []string {
	var entries []string
	for _, p := range posts {
		if len(p.created) == 0 || len(p.subtitle) == 0 {
			continue
		}
		e := fmt.Sprintf("%s %s: %s", p.created, p.name, p.subtitle)
		entries = append(entries, e)
	}
	sort.Strings(entries)
	return entries
}

func DumpAll(w io.StringWriter) {
	posts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	buf := &bytes.Buffer{}
	buf.WriteString("# https://notech.ie backup\n\n")
	entries := orderedEntries(posts)
	year := ""
	for _, e := range entries {
		if e[0:4] != year {
			fmt.Fprintf(buf, "\n%s entries:\n\n", e[0:4])
			year = e[0:4]
		}
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(buf, "- @#%s: %s\n", name, p.subtitle)
	}
	buf.WriteString("\n")
	for _, e := range entries {
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(buf, "!html <hr id=%s>\n\n", name)
		if bytes.Compare(p.content, p.rawcontent) == 0 {
			fmt.Fprintf(buf, "# %s: %s\n\nthis is not an ordinary post, see this content at @/%s.\n\n", p.name, p.subtitle, p.name)
			continue
		}
		buf.Write(p.rawcontent)
		buf.WriteString("\n\n")
	}
	w.WriteString(htmlHeader("notech.ie backup"))
	w.WriteString(markdown.Render(buf.String()))
	w.WriteString("</body></html>\n")
}

func genAutopages(posts map[string]post) {
	entries := orderedEntries(posts)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// frontpage
	httpmd := &bytes.Buffer{}
	httpmd.Write(posts["frontpage"].rawcontent)
	gopher := &bytes.Buffer{}
	for _, line := range strings.Split(string(posts["frontpage"].rawcontent), "\n") {
		fmt.Fprintf(gopher, "i%s\t.\t.\t0\n", line)
	}
	year := ""
	for _, e := range entries {
		if e[0:4] != year {
			fmt.Fprintf(httpmd, "\n%s entries:\n\n", e[0:4])
			fmt.Fprintf(gopher, "i\t.\t.\t0\ni%s entries:\t.\t.\t0\ni\t.\t.\t0\n", e[0:4])
			year = e[0:4]
		}
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		fmt.Fprintf(httpmd, "- @/%s\n", e[11:])
		fmt.Fprintf(gopher, "0/%s\t/%s\tnotech.ie\t70\n", e[11:], name)
	}
	httpresult := []byte(htmlHeader("notech.ie") + markdown.Render(httpmd.String()) + "</body></html>")
	p := post{
		name:        "frontpage",
		content:     httpresult,
		rawcontent:  gopher.Bytes(),
		contentType: http.DetectContentType(httpresult),
	}
	posts["frontpage"] = p

	// rss
	lastentries := entries
	if len(lastentries) > 7 {
		lastentries = lastentries[0:7]
	}
	rss := &bytes.Buffer{}
	rss.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
  <title>notech.ie</title>
  <description>a rambling personal blog of a techie</description>
  <link>http://notech.ie</link>
  <ttl>1380</ttl>
`)
	for _, e := range lastentries {
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(rss, "  <item><title>%s: %s</title>", p.name, p.subtitle)
		fmt.Fprintf(rss, "<link>https://notech.ie/%s</link></item>\n", p.name)
	}
	rss.WriteString("</channel>\n</rss>\n")
	p = post{
		name:        "rss",
		content:     rss.Bytes(),
		rawcontent:  rss.Bytes(),
		contentType: http.DetectContentType(rss.Bytes()),
	}
	posts["rss"] = p
}

func loadPosts() {
	log.Print("(re)loading posts")
	oldposts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	posts := map[string]post{}
	dirents, err := os.ReadDir(*postPath)
	if err != nil {
		log.Fatal(err)
	}
	for _, ent := range dirents {
		if p, ok := loadPost(ent.Name(), oldposts[ent.Name()]); ok {
			posts[ent.Name()] = p
		}
	}
	genAutopages(posts)
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache)), (unsafe.Pointer(&posts)))
	runtime.GC()
}

// Init starts the main loop for the post serving as a background goroutine.
func Init() {
	syscall.Mlockall(7) // never swap data to disk.
	loadPosts()
	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			loadPosts()
		}
	}()
}

func HandleHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	if len(path) == 0 && (strings.HasPrefix(req.Host, "notech.ie") || req.Proto == "gopher") {
		path = "frontpage"
	}
	posts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	p, ok := posts[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 not found"))
		return
	}
	log.Printf("serving %s %s", req.Proto, path)
	w.Header().Set("Content-Type", p.contentType)
	if req.Proto == "gopher" {
		w.Write(p.rawcontent)
	} else {
		w.Write(p.content)
	}
}
