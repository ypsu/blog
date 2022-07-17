// package posts implements the http/gopher handlers for serving my posts.
package posts

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"notech/markdown"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const commentCooldownMS = 3 * 60000

var postPath = flag.String("postpath", ".", "path to the posts")
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var titleRE = regexp.MustCompile(`(?:^#|\n!title) (\w+):? ([^\n]*)`)
var postsMutex sync.Mutex

type post struct {
	name, subtitle, created string
	content, rawcontent     []byte
	contentType             string
	lastmod                 time.Time
	commentsHash            uint64
}

var postsCache = &map[string]post{}

type comment struct {
	timestamp int64
	message   string
	response  string
}

var comments = map[string][]comment{}

func htmlHeader(title string, addrss bool) string {
	rss := ""
	if addrss {
		rss = "<link rel='alternate' type='application/rss+xml' title='rss feed for notech.ie' href=/rss>\n  "
	}
	return fmt.Sprintf(`<!doctype html><html lang=en><head>
  <title>%s</title>
  <meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'>
  %s<style>
    @media screen { body { max-width:50em;font-family:sans-serif } }
    blockquote { border-left: solid 0.25em darkgray; padding:0 0.5em; margin:1em 0 }
  </style>
</head><body>
`, title, rss)
}

func loadPost(name string, cachedPost post) (post, bool) {
	newPost := post{name: name}

	// check last modification.
	fileinfo, err := os.Stat(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return post{}, false
	}
	newPost.lastmod = fileinfo.ModTime()

	// check the hash of the comments.
	h := fnv.New64()
	for _, c := range comments[name] {
		binary.Write(h, binary.LittleEndian, c.timestamp)
		io.WriteString(h, c.message)
		io.WriteString(h, c.response)
	}
	newPost.commentsHash = h.Sum64()

	// return early if nothing changed.
	if newPost.lastmod == cachedPost.lastmod && newPost.commentsHash == cachedPost.commentsHash {
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
		buf.WriteString(htmlHeader(newPost.name, true))
		buf.WriteString(markdown.Render(string(newPost.content), false))
		buf.WriteString("<hr><p><a href=/>to the frontpage</a></p>\n")
		buf.WriteString("</body></html>\n")
		newPost.content = buf.Bytes()
	}

	if filepath.Ext(name) == ".js" {
		newPost.contentType = "application/javascript"
	} else {
		newPost.contentType = http.DetectContentType(newPost.content)
	}

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
	htmlre := regexp.MustCompile("(\n!html[^\n]*)+\n")
	buf.WriteString("\n")
	for _, e := range entries {
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(buf, "!html <hr id=%s>\n\n", name)
		if bytes.Compare(p.content, p.rawcontent) == 0 {
			fmt.Fprintf(buf, "# %s: %s\n\nthis is not an ordinary post, see this content at https://notech.ie/%s.\n\n", p.name, p.subtitle, p.name)
			continue
		}
		if bytes.Contains(p.rawcontent, []byte("\n!html")) {
			fmt.Fprintf(buf, "# %s: %s\n\n", p.name, p.subtitle)
			fmt.Fprintf(buf, "!html <p><i>this post has non-textual or interactive elements that were snipped from this backup page. see the full content at <a href=https://notech.ie/%s>https://notech.ie/%s</a>.</i></p>\n", p.name, p.name)
			c := p.rawcontent[bytes.IndexByte(p.rawcontent, byte('\n')):]
			c = htmlre.ReplaceAll(c, []byte("\n!html <p><i>[non-text content snipped]</i></p>\n"))
			buf.Write(c)
			buf.WriteString("\n\n")
			continue
		}
		buf.Write(p.rawcontent)
		buf.WriteString("\n\n")
	}
	w.WriteString(htmlHeader("notech.ie backup", false))
	md := markdown.Render(buf.String(), false)
	linkre := regexp.MustCompile("<a href='/([^']*)'>")
	w.WriteString(linkre.ReplaceAllString(md, "<a href='#$1'>"))
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
	httpresult := []byte(htmlHeader("notech.ie", true) + markdown.Render(httpmd.String(), false) + "</body></html>")
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
<?xml-stylesheet type="text/xsl" href="rss.xsl" media="screen"?>
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
		fmt.Fprintf(rss, "  <item><title>%s</title><description>%s</description>", p.name, p.subtitle)
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

func LoadPosts() {
	log.Print("(re)loading posts")
	postsMutex.Lock()

	oldposts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	posts := make(map[string]post, len(oldposts)+1)
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

	postsMutex.Unlock()
	runtime.GC()
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
