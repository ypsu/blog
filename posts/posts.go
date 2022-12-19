// package posts implements the http/gopher handlers for serving my posts.
package posts

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"notech/markdown"
	"notech/monitoring"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const commentCooldownMS = 3 * 60000

var postPath = flag.String("postpath", ".", "path to the posts")
var commentsFile = flag.String("commentsfile", "", "the backing file for comments.")
var commentsSalt = "the default salt string"
var lastCommentMS int64
var commentsInLastHour int
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

func staticRandom() (string, error) {
	// treat a device's uuid as the source of a static random string.
	dir, dev := "/dev/disk/by-uuid", "mmcblk0p2"
	uuids, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, uuid := range uuids {
		link, err := os.Readlink(filepath.Join(dir, uuid.Name()))
		if err != nil {
			continue
		}
		if filepath.Base(link) == dev {
			return uuid.Name(), nil
		}
	}
	return "", fmt.Errorf("%q not found", dev)
}

func Init() {
	salt, err := staticRandom()
	if err == nil {
		commentsSalt = salt
	} else {
		log.Printf("salt initialization failed: %v.", err)
	}
	log.Printf("salt is %q.", commentsSalt)
}

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
    textarea { width: 100%% }
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

		if *commentsFile != "" {
			buf.WriteString("<hr>\n")
			for i, c := range comments[name] {
				t := time.UnixMilli(c.timestamp).Format("2006-01-02")
				msg := markdown.Render(c.message, true)
				fmt.Fprintf(buf, "<p><b>comment #%d on %s</b></p><blockquote>%s</blockquote>\n", i+1, t, msg)
				if c.response != "" {
					fmt.Fprintf(buf, "<div style=margin-left:2em><p><b>comment #%d response from notech.ie</b></p><blockquote>%s</blockquote></div>\n", i+1, markdown.Render(c.response, false))
				}
			}
			buf.WriteString("<span id=hjs4comments>posting a comment requires javascript.</span>\n")
			buf.WriteString(`<span id=hnewcommentsection hidden>
  <p><b>new comment</b></p>
  <textarea id=hcommenttext rows=5></textarea>
  <p>
    <button id=hpreviewbutton>preview</button>
    <button id=hpostbutton>post</button>
    <span id=hcommentnote></span>
  </p>
  <blockquote id=hpreview></blockquote>
  <p>see <a href=/comments>@/comments</a> for the mechanics and ratelimits of commenting.</p>
  </span>
`)
			fmt.Fprintf(buf, "<script>const commentCooldownMS = %d</script>", commentCooldownMS)
			buf.WriteString("<script src=commentsapi.js></script>")
		}

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

func DumpAll() {
	posts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	recent, archive := &strings.Builder{}, &strings.Builder{}
	recent.WriteString("# https://notech.ie recent posts backup\n\n")
	recent.WriteString("\n!html older entries at <a href=archive.html>@/archive.html</a>.\n\n")
	archive.WriteString("# https://notech.ie old posts backup\n\n")
	entries := orderedEntries(posts)
	recentStart := strconv.Itoa(time.Now().Year() - 1)
	year := ""
	buf := archive
	for _, e := range entries {
		if e[0:4] >= recentStart {
			buf = recent
		}
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
	archive.WriteString("\n!html newer entries at <a href=index.html>@/index.html</a>.\n\n")
	recent.WriteString("\n")
	buf = archive
	for _, e := range entries {
		if e[0:4] >= recentStart {
			buf = recent
		}
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
		} else {
			buf.Write(p.rawcontent)
			buf.WriteString("\n\n")
		}
		fmt.Fprint(buf, "!html <hr>\n\n")
		for i, c := range comments[name] {
			t := time.UnixMilli(c.timestamp).Format("2006-01-02")
			msg := htmlre.ReplaceAllString(c.message, "\n!html <p><i>[non-text content snipped]</i></p>\n")
			fmt.Fprintf(buf, "!html <p id=%s.%d><b>comment #%s.%d on %s</b></p><blockquote>\n\n%s\n\n!html </blockquote>\n\n", name, i+1, name, i+1, t, msg)
			if c.response != "" {
				msg := htmlre.ReplaceAllString(c.response, "\n!html <p><i>[non-text content snipped]</i></p>\n")
				fmt.Fprintf(buf, "!html <div style=margin-left:2em><p><b>comment #%s.%d response from notech.ie</b></p><blockquote>\n\n%s\n\n!html </blockquote></div>\n\n", name, i+1, msg)
			}
		}
	}

	writefile := func(filename string, buf *strings.Builder) {
		w := &bytes.Buffer{}
		w.WriteString(htmlHeader("notech.ie backup", false))
		md := markdown.Render(buf.String(), false)
		linkre := regexp.MustCompile("<a href='/([^']*)'>")
		w.WriteString(linkre.ReplaceAllString(md, "<a href='#$1'>"))
		w.WriteString("</body></html>\n")
		os.WriteFile(filepath.Join(*postPath, filename), w.Bytes(), 0644)
	}
	writefile("index.html", recent)
	writefile("archive.html", archive)
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

	if *commentsFile != "" {
		commentsLog, err := os.ReadFile(*commentsFile)
		if err != nil {
			log.Fatalf("couldn't load comments: %v", err)
		}
		comments = map[string][]comment{}
		for _, line := range strings.Split(string(commentsLog), "\n") {
			if line == "" || strings.TrimSpace(line)[0] == '#' {
				continue
			}
			r := strings.NewReader(line)
			var tm int64
			var linetype string
			if n, err := fmt.Fscan(r, &tm, &linetype); n != 2 {
				log.Fatalf("couldn't parse comment line %q: %v", line, err)
			}
			if linetype == "comment" {
				var post, msg, resp string
				if n, err := fmt.Fscanf(r, "%s%q%q", &post, &msg, &resp); n != 3 {
					log.Fatalf("couldn't read comment from comment line %q: %v", line, err)
				}
				comments[post] = append(comments[post], comment{tm, msg, resp})
			} else {
				log.Fatalf("unrecognized linetype on comment line %q", line)
			}
		}
	}

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
	if path == "commentsapi" {
		handleCommentsAPI(w, req)
		return
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
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(p.rawcontent))
	} else {
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(p.content))
	}
}

var postRE = regexp.MustCompile("^[a-z0-9]+$")

func handleCommentsAPI(w http.ResponseWriter, r *http.Request) {
	if *commentsFile == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("comments service unavailable"))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	now := time.Now().UnixMilli()
	nowstr := strconv.FormatInt(now, 10)
	if msghash := r.Form.Get("sign"); msghash != "" {
		if len(msghash) != 64 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("msghash must be 64 bytes long"))
			return
		}
		code := strings.Join([]string{nowstr, msghash, commentsSalt}, "-")
		h := sha256.Sum256([]byte(code))
		signature := hex.EncodeToString(h[:]) + nowstr
		log.Print("signed a comment")
		fmt.Fprint(w, signature)
		return
	}

	msg := r.Form.Get("msg")
	if msg == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing message to post"))
		return
	}

	if len(msg) > 2000 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("message too long"))
		return
	}

	p := r.Form.Get("post")
	if !postRE.MatchString(p) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid post field"))
		return
	}

	signature := r.Form.Get("signature")
	if len(signature) != 64+13 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature format"))
		return
	}

	signatureTimeField := signature[64:]
	signatureTime, err := strconv.ParseInt(signatureTimeField, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("couldn't parse time format"))
		return
	}

	if now-signatureTime < commentCooldownMS {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("signature too recent"))
		return
	}

	msghash := sha256.Sum256([]byte(msg))
	code := strings.Join([]string{signatureTimeField, hex.EncodeToString(msghash[:]), commentsSalt}, "-")
	expectedSignature := sha256.Sum256([]byte(code))
	if signature != hex.EncodeToString(expectedSignature[:])+signatureTimeField {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("incorrect signature hash"))
		return
	}

	postsMutex.Lock()

	if lastCommentMS/3600000 == now/3600000 {
		if commentsInLastHour >= 4 {
			postsMutex.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("hourly global comment quota exceeded, try again an hour later"))
			monitoring.Alert(fmt.Sprintf("rejected comment to %s: %q", p, msg))
			return
		}
		commentsInLastHour++
	} else {
		lastCommentMS = now
		commentsInLastHour = 1
	}

	// persist the comment.
	f, err := os.OpenFile(*commentsFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("error opening %s: %v", *commentsFile, err)
	}
	fmt.Fprintf(f, "%s comment %s %q %q\n", nowstr, p, msg, "")
	if err := f.Close(); err != nil {
		log.Fatalf("error closing %s: %v", *commentsFile, err)
	}
	comments[p] = append(comments[p], comment{now, msg, ""})

	// regenerate the html.
	oldposts := *(*map[string]post)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache))))
	posts := make(map[string]post, len(oldposts))
	for k, v := range oldposts {
		posts[k] = v
	}
	var ok bool
	posts[p], ok = loadPost(p, oldposts[p])
	if !ok {
		log.Fatalf("couldn't regenerate post %s after a new comment", p)
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&postsCache)), (unsafe.Pointer(&posts)))

	postsMutex.Unlock()
	log.Printf("added a comment to the %s post", p)
	go runtime.GC()
}
