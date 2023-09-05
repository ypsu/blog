// package posts implements the http handlers for serving my posts.
package posts

import (
	"blog/markdown"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
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

	_ "embed"
)

const commentCooldownMS = 3 * 60000

var DumpallFlag = flag.Bool("dumpall", false, "if true dumps the backup version next to the posts.")
var postPath = flag.String("postpath", ".", "path to the posts")
var commentsFile = flag.String("commentsfile", "", "the backing file for comments.")
var commentsSalt = os.Getenv("COMMENTS_SALT")
var lastCommentMS int64
var commentsInLastHour int
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var titleRE = regexp.MustCompile(`(?:^#|\n!title) (\w+):? ([^\n]*)`)

type postContent struct {
	content      []byte
	gzipcontent  []byte
	contentType  string
	lastmod      time.Time
	commentsHash uint64
}

type post struct {
	name, subtitle, created string
	fromDisk                bool
	content                 atomic.Pointer[postContent]
}

type comment struct {
	timestamp int64
	message   string
	response  string
}

var postsMutex sync.Mutex
var postsCache atomic.Value
var comments = map[string][]comment{}

func Init() {
	if commentsSalt == "" {
		log.Print("missing $COMMENTS_SALT.")
	}
}

//go:embed header.thtml
var headerTemplate string

func htmlHeader(title string, addrss bool) string {
	rss := ""
	if addrss {
		rss = "\n  <link rel='alternate' type='application/rss+xml' title='rss feed for iio.ie' href=/rss>"
	}
	return fmt.Sprintf(headerTemplate, title, rss)
}

func compress(buf []byte) []byte {
	gz := &bytes.Buffer{}
	gzw, err := gzip.NewWriterLevel(gz, gzip.BestCompression)
	if err != nil {
		log.Fatalf("create gzip writer: %v.", err)
	}
	if n, err := gzw.Write(buf); err != nil || n != len(buf) {
		log.Fatalf("compress, wrote %d bytes: %v.", n, err)
	}
	if err := gzw.Close(); err != nil {
		log.Fatalf("close gzip writer: %v.", err)
	}
	return gz.Bytes()
}

func loadPost(p *post) *postContent {
	name := p.name
	content := p.content.Load()
	newcontent := &postContent{}

	if !p.fromDisk {
		return content
	}

	// check last modification.
	fileinfo, err := os.Stat(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return content
	}
	newcontent.lastmod = fileinfo.ModTime()

	// check the hash of the comments.
	h := fnv.New64()
	for _, c := range comments[name] {
		binary.Write(h, binary.LittleEndian, c.timestamp)
		io.WriteString(h, c.message)
		io.WriteString(h, c.response)
	}
	newcontent.commentsHash = h.Sum64()

	// return early if nothing changed.
	if content != nil && newcontent.lastmod == content.lastmod && newcontent.commentsHash == content.commentsHash {
		return content
	}

	// load the content.
	log.Printf("loading %s", name)
	rawcontent, err := os.ReadFile(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return content
	}
	newcontent.content = rawcontent

	// convert to html if it was a markdown file.
	if bytes.HasPrefix(newcontent.content, []byte("# ")) {
		buf := &bytes.Buffer{}
		buf.WriteString(htmlHeader(name, true))
		buf.WriteString(markdown.Render(string(newcontent.content), false))

		if *commentsFile != "" {
			buf.WriteString("<hr>\n")
			for i, c := range comments[name] {
				t := time.UnixMilli(c.timestamp).Format("2006-01-02")
				msg := markdown.Render(c.message, true)
				fmt.Fprintf(buf, "<div class=cComment id=c%d><p><b>comment <a href=#c%d>#%d</a> on %s</b></p><blockquote>%s</blockquote>\n", i+1, i+1, i+1, t, msg)
				if c.response != "" {
					fmt.Fprintf(buf, "<div style=margin-left:2em><p><b>comment #%d response from iio.ie</b></p><blockquote>%s</blockquote></div>\n", i+1, markdown.Render(c.response, false))
				}
				fmt.Fprint(buf, "</div>\n")
			}
			if !*DumpallFlag {
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
				fmt.Fprintf(buf, "<script>const commentCooldownMS = %d, commentPost = '%s', commentID = %d</script>\n", commentCooldownMS, name, len(comments[name]))
				buf.WriteString("<script src=commentsapi.js></script>")
			}
		}

		buf.WriteString("<hr><p><a href=/>to the frontpage</a></p>\n")
		buf.WriteString("</body></html>\n")
		newcontent.content = buf.Bytes()
	}

	if filepath.Ext(name) == ".js" {
		newcontent.contentType = "application/javascript"
	} else if filepath.Ext(name) == ".css" {
		newcontent.contentType = "text/css"
	} else {
		newcontent.contentType = http.DetectContentType(newcontent.content)
	}

	// pre-compute a compressed response.
	// but only if it saves at least 10% and at least 1KB.
	if gz := compress(newcontent.content); len(gz)+1024 < len(newcontent.content) && len(newcontent.content)*9 > len(gz)*10 {
		newcontent.gzipcontent = gz
	}

	p.content.Store(newcontent)
	return newcontent
}

func orderedEntries(posts map[string]*post) []string {
	var entries []string
	for _, p := range posts {
		if len(p.subtitle) == 0 {
			continue
		}
		e := fmt.Sprintf("%s %s: %s", p.created, p.name, p.subtitle)
		entries = append(entries, e)
	}
	sort.Strings(entries)
	return entries
}

func DumpAll() {
	linkre := regexp.MustCompile("<a href='/([a-z0-9]*)'>")
	writefile := func(filename, html string, addHeader bool) {
		w := &bytes.Buffer{}
		if addHeader {
			w.WriteString(htmlHeader("iio.ie backup", false))
			w.WriteString(linkre.ReplaceAllString(html, "<a href='#$1'>"))
			w.WriteString("</body></html>\n")
		} else {
			w.WriteString(linkre.ReplaceAllString(html, "<a href='$1.html'>"))
		}
		if err := os.WriteFile(filepath.Join(*postPath, filename), w.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}

	posts := postsCache.Load().(map[string]*post)
	recent, archive := &strings.Builder{}, &strings.Builder{}
	recent.WriteString("# https://iio.ie recent posts backup\n\n")
	recent.WriteString("\n!html older entries at <a href=archive.html>@/archive.html</a>.\n\n")
	archive.WriteString("# https://iio.ie old posts backup\n\n")
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
		if strings.HasSuffix(name, ".html") {
			continue
		}
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
		if strings.HasSuffix(name, ".html") {
			continue
		}
		p := posts[name]
		content := loadPost(p)
		rawcontent, err := os.ReadFile(filepath.Join(*postPath, name))
		if err != nil {
			log.Print("couldn't read %s: %v.", err)
			continue
		}
		fmt.Fprintf(buf, "!html <hr id=%s>\n", name)
		writefile(p.name+".html", string(content.content), false)
		fmt.Fprintf(buf, "!html <p style=font-weight:bold># <a href=#%s>%s</a>: %s</p>\n\n", p.name, p.name, p.subtitle)
		if bytes.Compare(content.content, rawcontent) == 0 {
			fmt.Fprintf(buf, "!html <p><i>this is not an ordinary post, see this content at <a href=%s.html>@/%s.html</a>.</i></p>\n\n", p.name, p.name)
			fmt.Fprintf(buf, "!pubdate %s\n\n", e[0:10])
			continue
		}
		c := rawcontent[bytes.IndexByte(rawcontent, byte('\n')):]
		if bytes.Contains(rawcontent, []byte("\n!html")) {
			fmt.Fprintf(buf, "!html <p><i>this post has non-textual or interactive elements that were snipped from this backup page. see the full content at <a href=%s.html>@/%s.html</a>.</i></p>\n", p.name, p.name)
			c = htmlre.ReplaceAll(c, []byte("\n!html <p><i>[non-text content snipped]</i></p>\n"))
		}
		buf.Write(c)
		buf.WriteString("\n\n")
		fmt.Fprint(buf, "!html <hr>\n\n")
		for i, c := range comments[name] {
			t := time.UnixMilli(c.timestamp).Format("2006-01-02")
			msg := htmlre.ReplaceAllString(c.message, "\n!html <p><i>[non-text content snipped]</i></p>\n")
			fmt.Fprintf(buf, "!html <p id=%s.c%d><b>comment <a href=#%s.c%d>#%s.%d</a> on %s</b></p><blockquote>\n\n%s\n\n!html </blockquote>\n\n", name, i+1, name, i+1, name, i+1, t, msg)
			if c.response != "" {
				msg := htmlre.ReplaceAllString(c.response, "\n!html <p><i>[non-text content snipped]</i></p>\n")
				fmt.Fprintf(buf, "!html <div style=margin-left:2em><p><b>comment #%s.%d response from iio.ie</b></p><blockquote>\n\n%s\n\n!html </blockquote></div>\n\n", name, i+1, msg)
			}
		}
	}
	writefile("index.html", markdown.Render(recent.String(), false), true)
	writefile("archive.html", markdown.Render(archive.String(), false), true)
}

func genAutopages(posts map[string]*post) {
	entries := orderedEntries(posts)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// frontpage
	httpmd := &bytes.Buffer{}
	frontpageHeader, err := os.ReadFile(filepath.Join(*postPath, "frontpage"))
	if err != nil {
		log.Printf("couldn't load the frontpage header: %v.", err)
	}
	httpmd.Write(frontpageHeader)
	year := ""
	for _, e := range entries {
		if e[0:4] != year {
			fmt.Fprintf(httpmd, "\n%s entries:\n\n", e[0:4])
			year = e[0:4]
		}
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		fmt.Fprintf(httpmd, "- @/%s\n", e[11:])
	}
	httpresult := []byte(htmlHeader("iio.ie", true) + markdown.Render(httpmd.String(), false) + "</body></html>")
	p := &post{name: "frontpage"}
	p.content.Store(&postContent{
		content:     httpresult,
		gzipcontent: compress(httpresult),
		contentType: http.DetectContentType(httpresult),
	})
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
  <title>iio.ie</title>
  <description>a rambling personal blog of a techie</description>
  <link>http://iio.ie</link>
  <ttl>1380</ttl>
`)
	for _, e := range lastentries {
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(rss, "  <item><title>%s</title><description>%s</description>", p.name, p.subtitle)
		if d, err := time.Parse("2006-01-02", p.created); err == nil {
			fmt.Fprintf(rss, "<pubDate>%s</pubDate>", d.Format(time.RFC1123))
		} else {
			log.Printf("post %s has invalid pubdate %q: %v.", p.name, p.created, err)
		}
		fmt.Fprintf(rss, "<link>https://iio.ie/%s</link></item>\n", p.name)
	}
	rss.WriteString("</channel>\n</rss>\n")
	p = &post{name: "rss"}
	p.content.Store(&postContent{
		content:     rss.Bytes(),
		contentType: http.DetectContentType(rss.Bytes()),
	})
	posts["rss"] = p
}

func LoadPosts() {
	postsMutex.Lock()
	defer postsMutex.Unlock()
	log.Print("(re)loading posts index.")

	if *commentsFile != "" {
		commentsLog, err := os.ReadFile(*commentsFile)
		if err != nil {
			log.Fatalf("couldn't load comments: %v", err)
		}
		var newCommentsLog []byte
		if !*DumpallFlag {
			newCommentsLog, err = os.ReadFile(*commentsFile + ".new")
			if err != nil {
				log.Fatalf("couldn't load new comments: %v", err)
			}
		}
		comments = map[string][]comment{}
		for _, line := range append(strings.Split(string(commentsLog), "\n"), strings.Split(string(newCommentsLog), "\n")...) {
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
				// in the case of a duplicate, take the most recent content.
				// this allows me to have overrides in the new comments file.
				cs := comments[post]
				if len(cs) == 0 || tm > cs[len(cs)-1].timestamp {
					comments[post] = append(cs, comment{tm, msg, resp})
				} else {
					i := sort.Search(len(cs), func(i int) bool { return cs[i].timestamp >= tm })
					if i == len(cs) || cs[i].timestamp != tm {
						comments[post] = append(cs, comment{tm, msg, resp})
					} else {
						cs[i] = comment{tm, msg, resp}
					}
				}
			} else {
				log.Fatalf("unrecognized linetype on comment line %q", line)
			}
		}
	}

	oldposts, _ := postsCache.Load().(map[string]*post)
	posts := make(map[string]*post, len(oldposts)+1)
	pubdates, err := os.ReadFile("pubdates.cache")
	if err != nil {
		log.Printf("pubdates.cache read failed, skipping load, err: %v.", err)
		return
	}
	for _, line := range bytes.Split(pubdates, []byte("\n")) {
		line := string(bytes.TrimSpace(line))
		if line == "" || line[0] == '#' {
			continue
		}
		var pubdate, fname, subtitle string
		_, err := fmt.Sscanf(line, "%s %s %q", &pubdate, &fname, &subtitle)
		if err != nil {
			log.Printf("scan of %q failed, skipping: %v.", line, err)
			continue
		}
		p := &post{
			created:  pubdate,
			name:     fname,
			subtitle: subtitle,
			fromDisk: true,
		}
		if op, ok := oldposts[fname]; ok {
			p.content.Store(op.content.Load())
			if op.fromDisk && p.content.Load() != nil {
				loadPost(p)
			}
		}
		posts[fname] = p
	}
	genAutopages(posts)
	postsCache.Store(posts)

	runtime.GC()
}

func HandleHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	if len(path) == 0 && (strings.HasPrefix(req.Host, "notech.ie")) {
		path = "frontpage"
	}
	if path == "commentsapi" {
		handleCommentsAPI(w, req)
		return
	}
	posts := postsCache.Load().(map[string]*post)
	p, ok := posts[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 not found"))
		return
	}
	content := p.content.Load()
	if content == nil {
		postsMutex.Lock()
		content = loadPost(p)
		postsMutex.Unlock()
	}
	w.Header().Set("Content-Type", content.contentType)
	w.Header().Set("Cache-Control", "max-age=3600")
	if content.gzipcontent != nil && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(content.gzipcontent))
	} else {
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(content.content))
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

	posts := postsCache.Load().(map[string]*post)

	p := r.Form.Get("post")
	if !postRE.MatchString(p) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid post field"))
		return
	}
	if p, found := posts[p]; !found || !p.fromDisk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("post not found"))
		return
	}

	idstr := r.Form.Get("id")
	id, err := strconv.Atoi(idstr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing id field or not a number"))
		return
	}
	postsMutex.Lock()
	commentsLen := len(comments[p])
	postsMutex.Unlock()
	if id > commentsLen {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("id too large"))
		return
	}
	// allow one additional comment to slip by before refusing the request.
	if id+1 < commentsLen {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("new comments appeared, save command and reload first"))
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
		code := strings.Join([]string{nowstr, p, idstr, msghash, commentsSalt}, "-")
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
	code := strings.Join([]string{signatureTimeField, p, idstr, hex.EncodeToString(msghash[:]), commentsSalt}, "-")
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
			log.Printf("rejected comment to %s: %q.", p, msg)
			return
		}
		commentsInLastHour++
	} else {
		lastCommentMS = now
		commentsInLastHour = 1
	}

	// persist the comment.
	fn := *commentsFile + ".new"
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("error opening %s: %v", fn, err)
	}
	fmt.Fprintf(f, "%s comment %s %q %q\n", nowstr, p, msg, "")
	if err := f.Close(); err != nil {
		log.Fatalf("error closing %s: %v", fn, err)
	}
	comments[p] = append(comments[p], comment{now, msg, ""})

	// regenerate the html.
	posts = postsCache.Load().(map[string]*post)
	loadPost(posts[p])

	postsMutex.Unlock()
	log.Printf("added a comment to the %s post", p)
	go runtime.GC()
}
