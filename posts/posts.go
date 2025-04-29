// Package posts implements the http handlers for serving my posts.
package posts

import (
	"blog/abname"
	"blog/alogdb"
	"blog/markdown"
	"blog/msgz"
	"blog/userapi"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"iter"
	"log"
	"maps"
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

const reactionPeriod = 12 * 3600 * 1000 // update reaction counts twice a day

var APIAddress string
var pullFlag = flag.Bool("pull", false, "do a git pull on startup.")
var apikey = os.Getenv("APIKEY") // api.iio.ie key
var postPath = flag.String("postpath", "docs", "path to the posts")
var commentsSalt = os.Getenv("SALT")
var lastCommentMS int64
var commentsInLastHour int
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var lastpullMS atomic.Int64
var htmlre = regexp.MustCompile("(\n!html[^\n]*)+\n")
var autoloadCount = 7 // number of entries to load automatically
var reactionKinds = []string{}
var reactionKindIDs = map[string]int{}

var now = func() int64 { return time.Now().UnixMilli() } // overridable for testing

type postContent struct {
	raw         []byte // this is the source, e.g. for building rss feed contents.
	content     []byte // this is what gets served, including comments.
	gzipcontent []byte // this is what gets served if compression is allowed, can be nil if it's not worth it.
	etag        string // the hash of content.
	contentType string
	lastmod     time.Time
	gentime     int64
}

// All fields except for the content can be accessed or modified only under a lock.
type post struct {
	name, subtitle, created string
	tags                    []string // tags of the post as specified in the file itself.
	generated               bool
	content                 atomic.Pointer[postContent]
}

var postsMutex sync.Mutex
var postsCache atomic.Value

func Init() {
	if commentsSalt == "" {
		log.Print("posts.MissingSaltEnvvar")
	}

	addreaction := func(r string) { reactionKinds, reactionKindIDs[r] = append(reactionKinds, r), len(reactionKinds) }
	addreaction("none")
	addreaction("like")
	addreaction("informative")
	addreaction("support")
	addreaction("congrats")
	addreaction("dislike")
	addreaction("unconvincing")
	addreaction("uninteresting")
	addreaction("unproductive")
	addreaction("unreadable")
	addreaction("unoriginal")
	addreaction("flag")
}

func live(nowTS, reactionTS int64) bool {
	cutoff := nowTS - nowTS%reactionPeriod - reactionPeriod/2
	return reactionTS <= cutoff
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

	if p.generated {
		return content
	}

	now := now()
	newcontent.gentime = now

	// Check last modification and return early if nothing changed.
	// Only do this if it hasn't been too long since the latest load.
	fileinfo, err := os.Stat(path.Join(*postPath, name))
	if err != nil {
		log.Print(err)
		return content
	}
	newcontent.lastmod = fileinfo.ModTime()
	if content != nil && content.gentime/reactionPeriod == now/reactionPeriod && fileinfo.ModTime() == content.lastmod {
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
		users := map[string]bool{}

		// Load the feedback.
		type key struct {
			cid, rid int
			userid   abname.ID
			kind     int
		}
		type commentreply struct {
			ts         int64
			user       string
			rawmessage string
		}
		type userreaction struct {
			kind    int
			rawnote string
		}
		comments := map[key]commentreply{key{}: commentreply{}} // dummy entry preallocated for the main post
		userreactions := map[key]userreaction{}
		reactionCounts := map[key]int{}
		reactionNotes := map[key][]string{}
		for _, e := range alogdb.DefaultDB.Get("feedback." + name) {
			parts := strings.SplitN(e.Text, " ", 4)
			if len(parts) != 4 {
				log.Printf("posts.InvalidFeedbackFormat name=%s ts=%d text=%q", name, e.TS, e.Text)
				continue
			}
			var cid, rid int
			if _, err := fmt.Sscanf(parts[1], "%d-%d", &cid, &rid); err != nil {
				log.Printf("posts.UnparseableFeedbackPosition name=%s ts=%d part=%q text=%q: %s", name, e.TS, parts[0], e.Text, err)
				continue
			}
			userid, err := abname.New(parts[2])
			if err != nil || userid == 0 {
				log.Printf("posts.InvalidUser name=%s ts=%d name=%q text=%q: %s", name, e.TS, parts[3], e.Text, err)
				continue
			}
			if parts[0] == "reaction" && live(now, e.TS) {
				reaction, note, _ := strings.Cut(parts[3], " ")
				if kind, found := reactionKindIDs[reaction]; found {
					userreactions[key{cid: cid, rid: rid, userid: userid}] = userreaction{kind: kind, rawnote: note}
				} else {
					log.Printf("posts.InvalidReaction ts=%d reaction=%q", e.TS, reaction)
				}
			} else if parts[0] == "comment" {
				comments[key{cid: cid, rid: rid}] = commentreply{e.TS, parts[2], parts[3]}
			}
		}
		for k, r := range userreactions {
			reactionCounts[key{cid: k.cid, rid: k.rid, kind: r.kind}]++
			if r.rawnote != "" {
				reactionNotes[key{cid: k.cid, rid: k.rid, kind: r.kind}] = append(reactionNotes[key{cid: k.cid, rid: k.rid, kind: r.kind}], r.rawnote)
			}
		}

		// Render the post.
		buf := &bytes.Buffer{}
		buf.WriteString(htmlHeader(name))
		buf.WriteString(markdown.Render(string(newcontent.content), false))
		buf.WriteString("<p class=cReactionLine data-id=0-0></p>\n")
		buf.WriteString("<hr>\n")
		buf.WriteString("<div id=eReactionbox hidden></div>")
		buf.WriteString("<div id=eUserinfobox hidden></div>")

		// Render the comments.
		if len(comments) >= 2 {
			fmt.Fprintf(buf, "<p><b>Comments:</b></p>")
		}
		var cid int
		for cid = 1; ; cid++ {
			topcomment, found := comments[key{cid: cid}]
			if !found {
				break
			}
			users[topcomment.user] = true
			t := time.UnixMilli(topcomment.ts).Format("2006-01-02")
			fmt.Fprintf(buf, "\n\n<div class=cComment id=c%d><p class=cReplyHeader><em><a href=#c%d>#c%d</a> by <span class=cPosterUsername>%s</span> on %s</em></p>\n%s\n", cid, cid, cid, topcomment.user, t, strings.TrimSpace(markdown.Render(topcomment.rawmessage, true)))
			fmt.Fprintf(buf, "<p class=cReactionLine data-id=%d-0></p></div>\n", cid)
			var rid int
			for rid = 1; ; rid++ {
				reply, found := comments[key{cid: cid, rid: rid}]
				if !found {
					break
				}
				users[reply.user] = true
				t := time.UnixMilli(reply.ts).Format("2006-01-02")
				fmt.Fprintf(buf, "\n\n<div class=cReply id=c%d-%d><p class=cReplyHeader><em><a href=#c%d-%d>#c%d-%d</a> by <span class=cPosterUsername>%s</span> on %s</em></p>\n%s\n", cid, rid, cid, rid, cid, rid, reply.user, t, strings.TrimSpace(markdown.Render(reply.rawmessage, true)))
				fmt.Fprintf(buf, "<p class=cReactionLine data-id=%d-%d></p></div>\n", cid, rid)
			}
			fmt.Fprintf(buf, "<div class='cReply cNeedsJS'><div></div><p><textarea placeholder='Write reply' id=eReplyEditor-%d-%d data-id=%d-%d rows=1></textarea></p><div></div></div>\n", cid, rid, cid, rid)
		}
		fmt.Fprintf(buf, "<p><b>Add new comment:</b></p>")
		fmt.Fprintf(buf, "<div class='cComment cNeedsJS'><div></div><p><textarea placeholder='Write new top level comment here...' id=eReplyEditor-%d-0 data-id=%d-0 rows=1></textarea></p><div></div></div>", cid, cid)
		fmt.Fprintf(buf, "<p class=cNoJSNote>(Adding a new comment or reply requires javascript.)</p>")
		fmt.Fprintf(buf, "<p id=eAccountpageLink></p>")

		buf.WriteString("<hr><p><a href=/>to the frontpage</a></p>\n")

		fmt.Fprintf(buf, "<script>\n  let PostName = '%s'\n  let PostRenderTS = %d\n", name, now)
		buf.WriteString("  let ReactionCounts = {\n")
		for cid := 0; ; cid++ {
			if _, found := comments[key{cid: cid}]; !found {
				break
			}
			for rid := 0; ; rid++ {
				if _, found := comments[key{cid: cid, rid: rid}]; !found {
					break
				}
				for kind, reaction := range reactionKinds {
					if cnt := reactionCounts[key{cid: cid, rid: rid, kind: kind}]; kind > 0 && cnt > 0 {
						fmt.Fprintf(buf, "    '%d-%d-%s': %d,\n", cid, rid, reaction, cnt)
					}
				}
			}
		}
		buf.WriteString("  }\n  ReactionNotes = {\n")
		for cid := 0; ; cid++ {
			if _, found := comments[key{cid: cid}]; !found {
				break
			}
			for rid := 0; ; rid++ {
				if _, found := comments[key{cid: cid, rid: rid}]; !found {
					break
				}
				for kind, reaction := range reactionKinds {
					if notes := reactionNotes[key{cid: cid, rid: rid, kind: kind}]; kind > 0 && len(notes) > 0 {
						fmt.Fprintf(buf, "    '%d-%d-%s': [", cid, rid, reaction)
						for _, note := range notes {
							fmt.Fprintf(buf, " %q,", note)
						}
						buf.WriteString(" ],\n")
					}
				}
			}
		}
		buf.WriteString("  }\n  let Userinfos = {\n")
		nowt := time.UnixMilli(now)
		for _, u := range slices.Sorted(maps.Keys(users)) {
			fmt.Fprintf(buf, "    %q: %q,\n", u, userapi.DefaultDB.Userinfo(u, nowt))
		}
		buf.WriteString("  }\n")
		buf.WriteString("  let iioui = null\n")
		buf.WriteString("  async function iioinit() {\n")
		buf.WriteString("    let iiomodule = await import(\"./iio.js\")\n")
		buf.WriteString("    iioui = iiomodule.iioui\n")
		buf.WriteString("    iiomodule.iio.Run(iioui.Init)\n")
		buf.WriteString("  }\n")
		buf.WriteString("  iioinit()\n")
		buf.WriteString("</script>\n")

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
	p := &post{name: "frontpage", generated: true}
	p.content.Store(&postContent{
		content:     httpresult,
		gzipcontent: compress(httpresult),
		contentType: http.DetectContentType(httpresult),
		etag:        hashBytes(httpresult),
	})
	posts["frontpage"] = p

	// rss
	lastentries := entries
	if len(lastentries) > autoloadCount {
		lastentries = lastentries[0:autoloadCount]
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
	p = &post{name: "rss", generated: true}
	p.content.Store(&postContent{
		content:     rss.Bytes(),
		gzipcontent: compress(rss.Bytes()),
		contentType: http.DetectContentType(rss.Bytes()),
		etag:        hashBytes(rss.Bytes()),
	})
	posts["rss"] = p
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
		}
		if tags != "" {
			p.tags = strings.Split(tags, " ")
		}
		if op, ok := oldposts[fname]; ok {
			p.content.Store(op.content.Load())
			if !op.generated && p.content.Load() != nil {
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
	path = strings.TrimPrefix(path, "new/") // TODO: remove after migration.
	if req.Host == "iio.ie" && path == "" {
		path = "frontpage"
	}
	if path == "feedbackapi" {
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
		http.Error(w, "posts.PostNotFound", http.StatusNotFound)
		return
	}
	content := p.content.Load()
	if content == nil || content.gentime/reactionPeriod != now()/reactionPeriod {
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
	if content.contentType == "text/html; charset=utf-8" {
		userapi.DefaultDB.Username(w, req) // clear session if user logged out
	}
	if content.gzipcontent != nil && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(content.gzipcontent)))
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(content.gzipcontent))
	} else {
		http.ServeContent(w, req, path, time.Time{}, bytes.NewReader(content.content))
	}
}

var postRE = regexp.MustCompile("^[a-z0-9]+$")

func handleUserdata(w http.ResponseWriter, r *http.Request) {
	p, tsparam := r.Form.Get("post"), r.Form.Get("ts")
	if p == "" || tsparam == "" {
		http.Error(w, "posts.MissingUserdataParams (must have both post and ts params)", http.StatusBadRequest)
		return
	}
	ts, err := strconv.ParseInt(tsparam, 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("posts.ParseTS ts=%q: %v", ts, err), http.StatusBadRequest)
		return
	}
	t := now()
	if ts > t {
		http.Error(w, fmt.Sprintf("posts.FutureUserdataTS ts=%d now=%d", ts, t), http.StatusBadRequest)
		return
	}
	if _, found := postsCache.Load().(map[string]*post)[p]; !found {
		http.Error(w, "posts.PostNotFound post="+p, http.StatusNotFound)
	}

	user := userapi.DefaultDB.Username(w, r)
	if user == "" {
		http.Error(w, "posts.NotLoggedIn", http.StatusUnauthorized)
		return
	}

	data := map[string]string{}

	for _, e := range alogdb.DefaultDB.Get("feedback." + p) {
		if !strings.HasPrefix(e.Text, "reaction ") {
			continue
		}
		parts := strings.SplitN(e.Text, " ", 4)
		if len(parts) != 4 {
			log.Printf("posts.InvalidReactionFormat post=%s ts=%d text=%q", p, e.TS, e.Text)
			continue
		}
		if parts[2] != user {
			continue
		}
		var cid, rid int
		if _, err := fmt.Sscanf(parts[1], "%d-%d", &cid, &rid); err != nil {
			log.Printf("posts.UnparseableReactionPosition post=%s ts=%d part=%q text=%q: %s", p, e.TS, parts[0], e.Text, err)
			continue
		}
		dataline := strings.TrimSpace(parts[3])
		if live(ts, e.TS) {
			data[parts[1]+"-live"] = dataline
			data[parts[1]+"-pending"] = dataline
		} else {
			data[parts[1]+"-pending"] = dataline
		}
	}

	for ref, reaction := range data {
		fmt.Fprintf(w, "reaction %s %s\n", ref, reaction)
	}
}

func handleCommentsAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("posts.ReadCommentsapiForm: %v", err), http.StatusBadRequest)
		return
	}
	action := r.Form.Get("action")
	if action == "" {
		http.Error(w, "posts.MissingActionParam", http.StatusBadRequest)
		return
	}
	if action == "userdata" {
		handleUserdata(w, r)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "posts.NotPOST", http.StatusBadRequest)
		return
	}
	if action != "previewcomment" && action != "comment" && action != "react" {
		http.Error(w, "posts.InvalidActionParam", http.StatusBadRequest)
		return
	}
	id := r.Form.Get("id")
	if id == "" {
		http.Error(w, "posts.MissingIDParam", http.StatusBadRequest)
		return
	}
	p, crid, ok := strings.Cut(id, ".")
	if !ok {
		http.Error(w, fmt.Sprintf("posts.InvalidIDSyntax id=%q", id), http.StatusBadRequest)
		return
	}

	posts := postsCache.Load().(map[string]*post)
	post, found := posts[p]
	if !found || post.generated {
		http.Error(w, fmt.Sprintf("posts.PostNotFound post=%q", p), http.StatusBadRequest)
		return
	}

	user := userapi.DefaultDB.Username(w, r)
	if user == "" {
		http.Error(w, "posts.NotLoggedIn", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("posts.ReadCommentsapiBody: %v", err), http.StatusBadRequest)
		return
	}
	if bytes.IndexByte(body, 0) != -1 {
		http.Error(w, "posts.UnsupportedZeroInBody", http.StatusBadRequest)
		return
	}
	if limit := 2000; len(body) > limit {
		http.Error(w, fmt.Sprintf("posts.CommentTooLong len=%d limit=%d", len(body), limit), http.StatusBadRequest)
		return
	}

	postsMutex.Lock()
	defer postsMutex.Unlock()

	if post.content.Load() == nil {
		loadPost(post)
	}

	var cid, rid int // comment and reply ID
	errmsg := "ok"
	if _, err := fmt.Sscanf(crid, "c%d-%d", &cid, &rid); err != nil {
		http.Error(w, fmt.Sprintf("posts.ParseCRID: %v", err), http.StatusBadRequest)
		return
	}
	if rid < 0 || cid < 0 {
		http.Error(w, "posts.NegativeCRID", http.StatusBadRequest)
		return
	}

	nowms := now()

	type key struct{ cid, rid int }
	exist := map[key]bool{key{0, 0}: true}
	for _, e := range alogdb.DefaultDB.Get("feedback." + p) {
		parts := strings.SplitN(e.Text, " ", 4)
		if len(parts) != 4 {
			log.Printf("posts.InvalidReactionFormat post=%s ts=%d text=%q", p, e.TS, e.Text)
			continue
		}
		var cid, rid int
		if _, err := fmt.Sscanf(parts[1], "%d-%d", &cid, &rid); err != nil {
			log.Printf("posts.UnparseableReactionPosition post=%s ts=%d part=%q text=%q: %s", p, e.TS, parts[0], e.Text, err)
			continue
		}
		exist[key{cid, rid}] = true
	}

	if action == "react" {
		if !exist[key{cid, rid}] {
			http.Error(w, "posts.NonexistentReactionTarget", http.StatusBadRequest)
			return
		}
		reaction := r.Form.Get("reaction")
		if reaction == "" {
			http.Error(w, "posts.MissingReactionParam", http.StatusBadRequest)
			return
		}
		if limit := 160; len(body) > limit {
			http.Error(w, fmt.Sprintf("posts.ReactionTooLong len=%d limit=%d", len(body), limit), http.StatusBadRequest)
			return
		}
		if bytes.IndexByte(body, '\n') != -1 {
			http.Error(w, "posts.UnsupportedNewlineInReaction", http.StatusBadRequest)
			return
		}
		if _, found := reactionKindIDs[reaction]; !found {
			http.Error(w, "posts.UnknownReactionParam reaction="+reaction, http.StatusBadRequest)
			return
		}

		logmsg := fmt.Sprintf("reaction %d-%d %s %s %s", cid, rid, user, reaction, bytes.TrimSpace(body))
		_, err := alogdb.DefaultDB.Add("feedback."+p, logmsg)
		if err != nil {
			http.Error(w, fmt.Sprintf("posts.PersistReaction: %v", err), http.StatusBadRequest)
			return
		}
		http.Error(w, "ok", http.StatusOK)
		msgz.Default.Printf("posts.Reacted commentid=%s user=%s reaction=%q commentary=%q", id, user, reaction, bytes.TrimSpace(body))
		return
	}

	if exist[key{cid, rid}] {
		errmsg = "posts.CommentAlreadyExist"
	}
	if rid == 0 && !exist[key{cid - 1, rid}] {
		errmsg = "posts.MissingPreviousComment"
	}
	if rid > 0 && !exist[key{cid, rid - 1}] {
		errmsg = "posts.MissingPreviousReply"
	}
	if action == "comment" && errmsg != "ok" {
		http.Error(w, errmsg, http.StatusConflict)
		return
	}

	// Sign the comment preview.
	if action == "previewcomment" {
		h := markdown.Render(string(body), true)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if errmsg != "ok" {
			fmt.Fprintf(w, "%s 0 - %s", errmsg, h)
			return
		}

		cost := len(exist)
		if rid == 0 {
			cost += cid * 5
		} else {
			cost += rid * 2
		}
		if strings.IndexByte(user, '-') == -1 {
			cost /= 2
		}
		validMS := nowms + int64(cost)*1000
		hasher := sha256.New224()
		io.WriteString(hasher, user)
		io.WriteString(hasher, "\000")
		io.WriteString(hasher, id)
		io.WriteString(hasher, "\000")
		io.WriteString(hasher, strconv.FormatInt(validMS, 10))
		io.WriteString(hasher, "\000")
		io.WriteString(hasher, commentsSalt)
		io.WriteString(hasher, "\000")
		hasher.Write(body)
		hashvalue := make([]byte, 0, sha256.Size224)
		sig := fmt.Sprintf("%s.%d", hex.EncodeToString(hasher.Sum(hashvalue)), validMS)

		fmt.Fprintf(w, "%s %d %s %s", errmsg, validMS-nowms, sig, h)
		msgz.Default.Printf("posts.PreviewedComment commentid=%s user=%s comment=%q", id, user, body)
		return
	}

	// Verify the preview signature.
	sig := r.Form.Get("sig")
	if sig == "" {
		http.Error(w, "posts.MissingSignatureParam", http.StatusBadRequest)
		return
	}
	sighash, sigtsString, _ := strings.Cut(sig, ".")
	sigts, err := strconv.ParseInt(sigtsString, 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("posts.ParseSignatureTimestamp sig=%q: %v", sig, err), http.StatusBadRequest)
		return
	}
	if sigts > nowms {
		http.Error(w, fmt.Sprintf("posts.PostTooEarly validfrom=%dms waittime=%dms", sigts, sigts-nowms), http.StatusBadRequest)
		return
	}
	hasher := sha256.New224()
	io.WriteString(hasher, user)
	io.WriteString(hasher, "\000")
	io.WriteString(hasher, id)
	io.WriteString(hasher, "\000")
	io.WriteString(hasher, sigtsString)
	io.WriteString(hasher, "\000")
	io.WriteString(hasher, commentsSalt)
	io.WriteString(hasher, "\000")
	hasher.Write(body)
	hashvalue := make([]byte, 0, sha256.Size224)
	vsig := hex.EncodeToString(hasher.Sum(hashvalue))
	if vsig != sighash {
		http.Error(w, "posts.InvalidSignature", http.StatusBadRequest)
		return
	}

	// Persist the comment.
	logmsg := fmt.Sprintf("comment %d-%d %s %s", cid, rid, user, body)
	if _, err := alogdb.DefaultDB.Add("feedback."+p, logmsg); err != nil {
		http.Error(w, fmt.Sprintf("posts.PersistComment: %v", err), http.StatusInternalServerError)
		return
	}
	msgz.Default.Printf("posts.AddedComment commentid=%s user=%s comment=%q", id, user, body)

	// Regenerate the html.
	post.content.Store(nil)
	loadPost(post)
	log.Printf("posts.NewComment post=%s", p)
	http.Error(w, "ok", http.StatusOK)
}

func hashBytes(b []byte) string {
	h := fnv.New64()
	h.Write(b)
	s := h.Sum64()
	return fmt.Sprintf(`"%016x"`, s)
}
