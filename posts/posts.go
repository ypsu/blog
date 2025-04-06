// package posts implements the http handlers for serving my posts.
package posts

import (
	"blog/markdown"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "embed"
)

const commentCooldownMS = 1 * 60000

var apiAddress = flag.String("api", "http://localhost:8787", "the address of the kv api for storing the new comments.")
var pullFlag = flag.Bool("pull", false, "do a git pull on startup.")
var apikey = os.Getenv("APIKEY") // api.iio.ie key
var postPath = flag.String("postpath", "docs", "path to the posts")
var commentsFile = flag.String("commentsfile", "comments.log", "the backing file for comments.")
var commentsSalt = os.Getenv("COMMENTS_SALT")
var lastCommentMS int64
var commentsInLastHour int
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var lastpullMS atomic.Int64
var htmlre = regexp.MustCompile("(\n!html[^\n]*)+\n")

type postContent struct {
	raw          []byte // this is the source, e.g. for building rss feed contents.
	content      []byte // this is what gets served, including comments.
	gzipcontent  []byte // this is what gets served if compression is allowed, can be nil if it's not worth it.
	etag         string // the hash of content.
	contentType  string
	lastmod      time.Time
	commentsHash uint64
}

type post struct {
	name, subtitle, created string
	tags                    []string // tags of the post as specified in teh file itself.
	fromDisk                bool
	content                 atomic.Pointer[postContent]
}

type commentSource int8

const (
	newcomment commentSource = iota
	committedcomment
	cloudcomment
)

type comment struct {
	timestamp int64
	message   string
	response  string
	source    commentSource
}

var postsMutex sync.Mutex
var postsCache atomic.Value
var comments = map[string][]comment{}

func Init() {
	if commentsSalt == "" {
		log.Print("missing $COMMENTS_SALT.")
	}
}

func Dump() iter.Seq2[string, string] {
	return func(yield func(k, v string) bool) {
		postsMutex.Lock()
		defer postsMutex.Unlock()
		posts := postsCache.Load().(map[string]*post)
		for name, post := range posts {
			pc := loadPost(post)
			if name == "rss" || bytes.HasPrefix(pc.content, []byte("<!doctype html>")) {
				yield(name, string(pc.content))
			}
		}
	}
}

func DumpRSS() iter.Seq2[string, string] {
	return func(yield func(k, v string) bool) {
		postsMutex.Lock()
		defer postsMutex.Unlock()
		posts := postsCache.Load().(map[string]*post)
		for name, post := range posts {
			pc := loadPost(post)
			if len(pc.raw) > 0 && bytes.HasPrefix(pc.content, []byte("<!doctype html>")) {
				yield(name, genRSSVersion(name, post.created, pc))
			}
		}
	}
}

//go:embed header.thtml
var headerTemplate string

func htmlHeader(title string) string {
	return fmt.Sprintf(headerTemplate, title)
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
	newcontent.raw = rawcontent
	newcontent.content = rawcontent

	// convert to html if it was a markdown file.
	if bytes.HasPrefix(newcontent.content, []byte("# ")) {
		buf := &bytes.Buffer{}
		buf.WriteString(htmlHeader(name))
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
			fmt.Fprintf(buf, "<script>const commentPost = '%s', commentID = %d</script>\n", name, len(comments[name]))
			buf.WriteString("<script src=commentsapi.js></script>")
		}

		buf.WriteString("<hr><p><a href=/>to the frontpage</a></p>\n")
		buf.WriteString("</body></html>\n")
		newcontent.content = buf.Bytes()
	}

	if filepath.Ext(name) == ".js" {
		newcontent.contentType = "application/javascript; charset=utf-8"
	} else if filepath.Ext(name) == ".css" {
		newcontent.contentType = "text/css; charset=utf-8"
	} else {
		newcontent.contentType = http.DetectContentType(newcontent.content)
	}

	// pre-compute a compressed response.
	// but only if it saves at least 10% and at least 1KB.
	if gz := compress(newcontent.content); len(gz)+1024 < len(newcontent.content) && len(newcontent.content)*9 > len(gz)*10 {
		newcontent.gzipcontent = gz
	}
	newcontent.etag = hashBytes(newcontent.content)

	p.content.Store(newcontent)
	return newcontent
}

func orderedEntries(posts map[string]*post) []string {
	today := time.Now().UTC().Format("2006-01-02")
	var entries []string
	for _, p := range posts {
		if len(p.subtitle) == 0 || p.created > today {
			continue
		}
		e := fmt.Sprintf("%s %s: %s", p.created, p.name, p.subtitle)
		entries = append(entries, e)
	}
	sort.Strings(entries)
	if len(entries) == 0 || posts["theend"] == nil {
		return entries
	}

	// add the automated the-end post.
	lastpost, err := time.Parse("2006-01-02", entries[len(entries)-1][:10])
	if err != nil {
		return entries
	}
	deadline := lastpost.Add(92 * 24 * time.Hour).Format("2006-01-02")
	if today >= deadline {
		posts["theend"].created = deadline
		entries = append(entries, fmt.Sprintf("%s theend: %s", deadline, posts["theend"].subtitle))
	}
	return entries
}

func genRSSVersion(name, pubdate string, content *postContent) (html string) {
	md := &strings.Builder{}
	if bytes.Compare(content.content, content.raw) == 0 {
		fmt.Fprintf(md, "!html <p><i>this is not an ordinary post, see this content at <a href=https://iio.ie/%s>@/%s</a>.</i></p>\n\n", name, name)
		fmt.Fprintf(md, "!pubdate %s\n\n", pubdate)
	} else {
		c := content.raw[bytes.IndexByte(content.raw, byte('\n')):]
		if bytes.Contains(c, []byte("\n!html")) {
			fmt.Fprintf(md, "!html <p><i>this post has non-textual or interactive elements that were snipped from rss. see the full content at <a href=https://iio.ie/%s>@/%s</a>.</i></p>\n", name, name)
			c = htmlre.ReplaceAll(c, []byte("\n!html <p><i>[non-text content snipped]</i></p>\n"))
		}
		fmt.Fprintf(md, "%s", c)
	}
	return strings.ReplaceAll(markdown.Render(md.String(), false), "<a href='/", "<a href='https://iio.ie/")
}

func genAutopages(posts map[string]*post) {
	entries := orderedEntries(posts)
	slices.Reverse(entries)

	// frontpage
	httpmd := &bytes.Buffer{}
	frontpageHeader, err := os.ReadFile(filepath.Join(*postPath, "frontpage"))
	if err != nil {
		log.Printf("couldn't load the frontpage header: %v.", err)
	}
	httpmd.Write(frontpageHeader)
	year := ""
	tags := map[string][]string{}
	for _, e := range entries {
		if e[0:4] != year {
			fmt.Fprintf(httpmd, "\n%s entries:\n\n", e[0:4])
			year = e[0:4]
		}
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		fmt.Fprintf(httpmd, "- @/%s\n", e[11:])
		for _, tag := range posts[name].tags {
			tags[tag] = append(tags[tag], name)
		}
	}
	fmt.Fprint(httpmd, "\n!html <script>let tags = { ")
	tagnames := make([]string, 0, len(tags))
	for tag := range tags {
		tagnames = append(tagnames, tag)
	}
	slices.Sort(tagnames)
	for _, tag := range tagnames {
		posts := tags[tag]
		fmt.Fprintf(httpmd, "%s:[", tag)
		for _, p := range posts {
			fmt.Fprintf(httpmd, "'%s',", p)
		}
		fmt.Fprintf(httpmd, "], ")
	}
	fmt.Fprintf(httpmd, "}</script>\n")
	fmt.Fprint(httpmd, "!html <p id=hFilterMessage>filtered entries:</p><ul id=hSelection hidden></ul><script src=frontpage.js></script>")
	httpresult := []byte(htmlHeader("iio.ie") + markdown.Render(httpmd.String(), false) + "</body></html>")
	p := &post{name: "frontpage"}
	p.content.Store(&postContent{
		content:     httpresult,
		gzipcontent: compress(httpresult),
		contentType: http.DetectContentType(httpresult),
		etag:        hashBytes(httpresult),
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
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
  <title>iio.ie</title>
  <description>a rambling personal blog of a techie</description>
  <link>http://iio.ie</link>
  <ttl>1380</ttl>
  <atom:link rel="self" href="https://iio.ie/rss" type="application/rss+xml"/>
`)
	for _, e := range lastentries {
		name := strings.Fields(e)[1]
		name = name[:len(name)-1]
		p := posts[name]
		fmt.Fprintf(rss, "  <item><title>%s: %s</title>", p.name, p.subtitle)
		if d, err := time.Parse("2006-01-02", p.created); err == nil {
			fmt.Fprintf(rss, "<pubDate>%s</pubDate>", d.Format(time.RFC1123Z))
			content := p.content.Load()
			if content == nil {
				content = loadPost(p)
			}
			fmt.Fprintf(rss, "<description>%s</description>", html.EscapeString(genRSSVersion(p.name, e[:10], content)))
		} else {
			log.Printf("post %s has invalid pubdate %q: %v.", p.name, p.created, err)
		}
		fmt.Fprintf(rss, "<link>https://iio.ie/%s</link><guid>https://iio.ie/%s</guid></item>\n", p.name, p.name)
	}
	rss.WriteString("</channel>\n</rss>\n")
	p = &post{name: "rss"}
	p.content.Store(&postContent{
		content:     rss.Bytes(),
		gzipcontent: compress(rss.Bytes()),
		contentType: http.DetectContentType(rss.Bytes()),
		etag:        hashBytes(rss.Bytes()),
	})
	posts["rss"] = p
}

func commentAtTime(post string, tm int64) *comment {
	cs := comments[post]
	if len(cs) == 0 || tm > cs[len(cs)-1].timestamp {
		comments[post] = append(cs, comment{timestamp: tm, source: newcomment})
		return &comments[post][len(cs)]
	}
	i := sort.Search(len(cs), func(i int) bool { return cs[i].timestamp >= tm })
	if i == len(cs) || cs[i].timestamp != tm {
		comments[post] = append(cs, comment{timestamp: tm, source: newcomment})
		sort.Slice(comments[post][i:], func(a, b int) bool {
			return comments[post][i+a].timestamp < comments[post][i+b].timestamp
		})
		return &comments[post][i]
	}
	return &cs[i]
}

func gitpull(w io.Writer) {
	prev, now := lastpullMS.Load(), time.Now().UnixMilli()
	if now-prev < 60_000 {
		log.Printf("skipping git pull, too soon.")
		fmt.Fprintf(w, "skipped: too soon")
		return
	}
	if !lastpullMS.CompareAndSwap(prev, now) {
		log.Printf("skipping git pull, conflict.")
		fmt.Fprintf(w, "skipped: conflict with another pull")
		return
	}
	cmd := exec.Command("git", "pull")
	stdout, err := cmd.Output()
	if err != nil {
		log.Printf("git pull failed: %v, stdout:\n%s", err, stdout)
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			log.Printf("git pull stderr:\n%s", ee.Stderr)
		}
		fmt.Fprintf(w, "git pull failed: %v", err)
		return
	}
	log.Printf("git pull succeeded, stdout:\n%s", stdout)
	fmt.Fprintf(w, "ok")
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
		if len(comments) == 0 {
			// this is the first time running, git pull and fetch not yet commited comments from cloudflare.
			wg := sync.WaitGroup{}
			if *pullFlag {
				log.Print("git pulling")
				wg.Add(1)
				go func() {
					gitpull(io.Discard)
					wg.Done()
				}()

				// and periodically rerender the frontpage to pick up future posts.
				go func() {
					for {
						time.Sleep(6 * time.Hour)
						LoadPosts()
					}
				}()
			}
			log.Print("fetching comments from the api server")
			body, err := callAPI("GET", "/api/kvall?prefix=comments", "")
			if err != nil {
				log.Printf("failed to load the new logs: %v.", err)
			}
			var tm int64
			var post, msg string
			var cnt int
			for _, line := range strings.Split(body, "\n") {
				line := strings.TrimSpace(line)
				if line == "" || line[0] == '#' {
					continue
				}
				if _, err := fmt.Sscanf(line, "%d%s%q", &tm, &post, &msg); err != nil {
					log.Printf("couldn't parse comment %q: %v", line, err)
					continue
				}
				*commentAtTime(post, tm) = comment{tm, msg, "", cloudcomment}
				cnt++
				if tm/3600_000 == lastCommentMS/3600_000 {
					commentsInLastHour++
				} else if tm/3600_000 > lastCommentMS/3600_000 {
					commentsInLastHour = 1
				}
				if tm > lastCommentMS {
					lastCommentMS = tm
				}
			}
			log.Printf("loaded %d comments from the api server.", cnt)
			wg.Wait()
		}
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
				c := commentAtTime(post, tm)
				if c.source == cloudcomment {
					// delete from cloud if the comment got committed.
					go func(post string, tm int64) {
						log.Printf("deleting %s comment %d.", post, tm)
						_, err := callAPI("DELETE", fmt.Sprintf("/api/kv?key=comments.%d", tm), "")
						if err != nil {
							log.Printf("deleting %s comment %d failed: %v", post, tm, err)
						}
					}(post, tm)
				}
				*c = comment{tm, msg, resp, committedcomment}
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
		var pubdate, fname, subtitle, tags string
		_, err := fmt.Sscanf(line, "%s %s %q %q", &fname, &pubdate, &subtitle, &tags)
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
		if tags != "" {
			p.tags = strings.Split(tags, " ")
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
	if req.Host == "iio.ie" && path == "" {
		path = "frontpage"
	}
	if path == "commentsapi" {
		handleCommentsAPI(w, req)
		return
	}
	if req.Host == "notech.ie" {
		if path == "rss" {
			path = "badrss"
		} else {
			w.WriteHeader(http.StatusMovedPermanently)
			f := `<body>notech.ie is no more. go here: <a href="https://iio.ie%s">https://iio.ie%s</a>. see <a href=https://iio.ie/rebrand>@/rebrand</a> for details.</body>`
			fmt.Fprintf(w, f, html.EscapeString(req.URL.Path), req.URL.Path)
			return
		}
	}
	if path == "reloadposts" {
		gitpull(w)
		LoadPosts()
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
	if req.Header.Get("If-None-Match") == content.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if content.etag != "" {
		w.Header().Set("ETag", content.etag)
	}
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

	if r.Form.Has("new") {
		postsMutex.Lock()
		for post, cs := range comments {
			for _, c := range cs {
				if c.source == cloudcomment {
					fmt.Fprintf(w, "%d comment %s %q %q\n", c.timestamp, post, c.message, c.response)
				}
			}
		}
		postsMutex.Unlock()
		return
	}

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
	var lastMessage string
	if commentsLen > 0 {
		lastMessage = comments[p][commentsLen-1].message
	}
	postsMutex.Unlock()
	// allow one additional comment to slip by before refusing the request.
	if id+1 < commentsLen {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("new comments appeared, reload and retry"))
		return
	}

	now := time.Now().UTC().UnixMilli()
	nowstr := strconv.FormatInt(now, 10)
	validfrom := now + commentCooldownMS
	validfromStr := strconv.FormatInt(validfrom, 10)
	if msghash := r.Form.Get("sign"); msghash != "" {
		if len(msghash) != 64 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("msghash must be 64 bytes long"))
			return
		}
		code := strings.Join([]string{validfromStr, p, idstr, msghash, commentsSalt}, "-")
		h := sha256.Sum256([]byte(code))
		signature := hex.EncodeToString(h[:]) + "." + validfromStr
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
	if msg == lastMessage {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("duplicate message"))
		return
	}

	if len(msg) > 2000 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("message too long"))
		return
	}

	signature := r.Form.Get("signature")
	if len(signature) != 64+1+13 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid signature format"))
		return
	}

	signatureTimeField := signature[65:]
	signatureTime, err := strconv.ParseInt(signatureTimeField, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("couldn't parse time format"))
		return
	}

	if now < signatureTime {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("signature too recent"))
		return
	}

	msghash := sha256.Sum256([]byte(msg))
	code := strings.Join([]string{signatureTimeField, p, idstr, hex.EncodeToString(msghash[:]), commentsSalt}, "-")
	expectedSignature := sha256.Sum256([]byte(code))
	if signature != hex.EncodeToString(expectedSignature[:])+"."+signatureTimeField {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("incorrect signature hash"))
		return
	}

	postsMutex.Lock()

	for now <= lastCommentMS {
		now++
	}
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
	data := fmt.Sprintf("%s %s %q\n", nowstr, p, msg)
	u := fmt.Sprintf("/api/kv?key=comments.%s", nowstr)
	if _, err := callAPI("PUT", u, data); err != nil {
		commentsInLastHour--
		postsMutex.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "persist comment: %v", err)
		return
	}
	comments[p] = append(comments[p], comment{now, msg, "", cloudcomment})

	// regenerate the html.
	posts = postsCache.Load().(map[string]*post)
	loadPost(posts[p])

	postsMutex.Unlock()
	log.Printf("added a comment to the %s post", p)
	go runtime.GC()
}

// callAPI invokes the specific api method over http.
// the response's body is returned iff error is nil.
func callAPI(method, url, body string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, *apiAddress+url, strings.NewReader(body))
	if err != nil {
		err := fmt.Errorf("http.NewRequestWithContext: %v", err)
		log.Print(err)
		return "", err
	}

	req.Header.Add("apikey", apikey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err := fmt.Errorf("http.Do: %v", err)
		log.Print(err)
		return "", err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		err := fmt.Errorf("io.ReadAll(resp.Body): %v", err)
		log.Print(err)
		return "", err
	}

	if err := resp.Body.Close(); err != nil {
		fmt.Printf("%s api call uncleanly finished: resp.Body.Close: %v", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, respBody)
		log.Print(err)
		return "", err
	}

	return string(respBody), nil
}

func hashBytes(b []byte) string {
	h := fnv.New64()
	h.Write(b)
	s := h.Sum64()
	return fmt.Sprintf(`"%016x"`, s)
}
