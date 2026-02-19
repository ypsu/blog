package posts

import (
	"blog/abname"
	"blog/alogdb"
	"blog/testwriter"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ypsu/efftesting/efft"
)

type testResponse struct {
	code int
	hdr  http.Header
	buf  bytes.Buffer
}

func (r *testResponse) Header() http.Header  { return r.hdr }
func (r *testResponse) WriteHeader(code int) { r.code = code }
func (r *testResponse) Write(buf []byte) (int, error) {
	if r.code == 0 {
		r.code = 200
	}
	return r.buf.Write(buf)
}

func TestHandlers(t *testing.T) {
	tm := int64(100000)
	now = func() int64 { tm++; return tm }
	efft.Init(t)
	fh := efft.Must1(os.Open("/dev/null"))
	defer fh.Close()
	efft.Override(&alogdb.DefaultDB, efft.Must1(alogdb.NewForTesting(fh)))

	*postPath = "."
	_, outputs := testwriter.Data(t)
	for k := range outputs {
		delete(outputs, k)
	}
	LoadPosts()

	query := func(page string) {
		url, err := url.Parse(page)
		if err != nil {
			t.Fatal(err)
		}
		request := &http.Request{URL: url}
		response := &testResponse{hdr: http.Header{}}
		HandleHTTP(response, request)
		ctype := fmt.Sprintf("Content-Type: %s\n", response.hdr["Content-Type"])
		outputs[page] = ctype + response.buf.String()
	}

	dirents, err := os.ReadDir(".")
	if err != nil {
		log.Fatal(err)
	}
	for _, ent := range dirents {
		if !strings.HasPrefix(ent.Name(), "sample") {
			continue
		}
		query(ent.Name())
	}
	query("frontpage")
	query("latest")
	query("rss")
}

func TestCommentHandler(t *testing.T) {
	efft.Init(t)

	tm := int64(100000)
	now = func() int64 { tm++; return tm }
	alogdb.Now = now

	logfile := filepath.Join(t.TempDir(), "log")
	fh := efft.Must1(os.OpenFile(logfile, os.O_RDWR|os.O_CREATE, 0644))
	defer fh.Close()
	efft.Override(&alogdb.DefaultDB, efft.Must1(alogdb.NewForTesting(fh)))

	*postPath = "."
	efft.Override(&autoloadCount, 1)
	postsCache.Store(map[string]*post{})
	abname.Init()
	Init()
	LoadPosts()

	var lastsig string
	query := func(path, body string) string {
		hdr := http.Header{}
		// Sig generated via `echo -n "testuser-guest " | sha256sum`.
		hdr.Set("Cookie", "session=testuser-guest.956896cf4b343b373345f65d79fc4bf08aa35d6594204ea4ef4b97e7b4253b82")
		request := &http.Request{
			Method: "POST",
			URL:    efft.Must1(url.Parse("http://localhost" + path)),
			Header: hdr,
			Body:   io.NopCloser(strings.NewReader(body)),
		}
		response := &testResponse{hdr: http.Header{}}
		HandleHTTP(response, request)
		s := strings.TrimSpace(fmt.Sprintf("%d %s", response.code, response.buf.Bytes()))
		if parts := strings.Split(s, " "); len(parts) >= 4 {
			lastsig = parts[3]
		}
		return s
	}

	efft.Effect(query("/feedbackapi", "")).Equals("400 posts.MissingActionParam")
	efft.Effect(query("/feedbackapi?action=bogus", "")).Equals("400 posts.InvalidActionParam")

	const waittime = 600000
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c1-0", "Hello top comment.")).Equals("200 ok 6000 4e3f1672f4dc8ae3e9eb597062c9656fddb2592b15c50d0ca516e57b.106003 <p>Hello top comment.</p>")
	sig1 := lastsig
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0", "Hello top comment.")).Equals("400 posts.MissingSignatureParam")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0&sig=bogus", "Hello top comment.")).Equals("400 posts.ParseSignatureTimestamp sig=\"bogus\": strconv.ParseInt: parsing \"\": invalid syntax")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0&sig="+sig1, "bogus")).Equals("400 posts.PostTooEarly validfrom=106003ms waittime=5997ms")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0&sig=abcd.1234", "Hello top comment.")).Equals("400 posts.InvalidSignature")
	tm += waittime
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0&sig="+sig1, "bogus")).Equals("400 posts.InvalidSignature")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-0&sig="+sig1, "Hello top comment.")).Equals("200 ok")

	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c1-1", "Hello reply.")).Equals("200 ok 4000 a0572f3661139be649c1961ca082d7b2f73caca6abeac2d8094ed00d.704012 <p>Hello reply.</p>")
	sig2 := lastsig
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c1-1", "Hello another reply.")).Equals("200 ok 4000 15b769ebede5c5eb5d522fe4ba2ad56524f4a2548872a68a8877ca7d.704013 <p>Hello another reply.</p>")
	sig2race := lastsig
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c2-0", "Hello another top comment.")).Equals("200 ok 12000 769843ee6dc63c2c1e4b1f4e4aa83957f639134ad1ad534c91f115cc.712014 <p>Hello another top comment.</p>")
	sig3 := lastsig
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c4-0", "Hello bad top comment.")).Equals("200 posts.MissingPreviousComment 0 - <p>Hello bad top comment.</p>")
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegooddup.c0-0", "Hello top comment in another post.")).Equals("200 posts.MissingPreviousComment 0 - <p>Hello top comment in another post.</p>")
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegooddup.c1-0", "Hello top comment in another post.")).Equals("200 ok 6000 7bba237ded0d478520d15fddd36b99d3f1ca4f68452b4edca8b52d13.706018 <p>Hello top comment in another post.</p>")
	sig6 := lastsig
	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c1-0", "Hello bad top comment.")).Equals("200 posts.CommentAlreadyExist 0 - <p>Hello bad top comment.</p>")
	tm += waittime
	efft.Effect(query("/feedbackapi?action=comment&id=samplegooddup.c1-0&sig="+sig6, "Hello top comment in another post.")).Equals("200 ok")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c2-0&sig="+sig3, "Hello another top comment.")).Equals("200 ok")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-1&sig="+sig2, "Hello reply.")).Equals("200 ok")
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-1&sig="+sig2race, "Hello another reply.")).Equals("409 posts.CommentAlreadyExist")

	efft.Effect(query("/feedbackapi?action=previewcomment&id=samplegood.c1-2", "Another reply here.")).Equals("200 ok 8000 1511f4d220941d91283c04736def0e3139f4c312ef7491fe11b6e585.1308030 <p>Another reply here.</p>")
	tm += waittime
	efft.Effect(query("/feedbackapi?action=comment&id=samplegood.c1-2&sig="+lastsig, "Another reply here.")).Equals("200 ok")

	efft.Effect(query("/feedbackapi?action=react&id=nonexistent.c0-0&reaction=like", "")).Equals("400 posts.PostNotFound post=\"nonexistent\"")
	efft.Effect(query("/feedbackapi?action=react&id=samplegood.c9-9&reaction=like", "")).Equals("400 posts.NonexistentReactionTarget")
	efft.Effect(query("/feedbackapi?action=react&id=samplegood.c1-9&reaction=like", "")).Equals("400 posts.NonexistentReactionTarget")
	efft.Effect(query("/feedbackapi?action=react&id=samplegood.c0-0&reaction=dislike", "")).Equals("200 ok")
	efft.Effect(query("/feedbackapi?action=react&id=samplegood.c1-1&reaction=like", "This is a note!")).Equals("200 ok")

	toDeterministicData := func(s string) string {
		_, body, _ := strings.Cut(s, " ")
		lines := strings.Split(body, "\n")
		slices.Sort(lines)
		return strings.Join(lines, "\n")
	}
	efft.Effect(toDeterministicData(query("/feedbackapi?action=userdata&post=samplegood&ts="+strconv.FormatInt(tm, 10), ""))).Equals(`
		reaction 0-0-pending dislike
		reaction 1-1-pending like This is a note!`)
	baseRender := loadPost(postsCache.Load().(map[string]*post)["samplegood"]).content
	tm += 24 * 3600 * 1000
	efft.Effect(toDeterministicData(query("/feedbackapi?action=userdata&post=samplegood&ts="+strconv.FormatInt(tm, 10), ""))).Equals(`
		reaction 0-0-live dislike
		reaction 0-0-pending dislike
		reaction 1-1-live like This is a note!
		reaction 1-1-pending like This is a note!`)

	efft.Effect(strings.ReplaceAll(string(efft.Must1(os.ReadFile(logfile))), "\000", "")).Equals(`
		700010 feedback.samplegood comment 1-0 testuser-guest Hello top comment.
		1300021 feedback.samplegooddup comment 1-0 testuser-guest Hello top comment in another post.
		1300024 feedback.samplegood comment 2-0 testuser-guest Hello another top comment.
		1300027 feedback.samplegood comment 1-1 testuser-guest Hello reply.
		1900032 feedback.samplegood comment 1-2 testuser-guest Another reply here.
		1900037 feedback.samplegood reaction 0-0 testuser-guest dislike 
		1900039 feedback.samplegood reaction 1-1 testuser-guest like This is a note!
	`)

	laterRender := loadPost(postsCache.Load().(map[string]*post)["samplegood"]).content
	efft.Effect(baseRender).Equals(`
		<!doctype html><html lang=en><head>
		  <title>samplegood</title>
		  <meta charset=utf-8>
		  <meta name=viewport content='width=device-width,initial-scale=1'>
		  <style>:root{color-scheme:light dark}</style>
		  <link rel=icon href=favicon.ico>
		  <link rel=stylesheet href=style.css>
		  <link rel=alternate type=application/rss+xml title=iio.ie href=rss>
		</head><body>
		<pre id=eError class=cbgNegative hidden></pre>
		<p style=font-weight:bold># samplegood: a sample post.</p>

		<p>lorem ipsum.</p>

		<p>here&#39;s some markdown reference: <a href='/samplehtml'>@/samplehtml</a>.</p>

		<p>some custom html.</p>


		<p><i>published on 2021-06-03</i></p>

		<p class=cReactionLine data-id=0-0></p>
		<hr>
		<div id=eReactionbox hidden></div><div id=eUserinfobox hidden></div><p><b>Comments:</b></p>

		<div class=cComment id=c1><p class=cReplyHeader><em><a href=#c1>#c1</a> by <span class=cPosterUsername>testuser-guest</span> on 1970-01-01</em></p>
		<p>Hello top comment.</p>
		<p class=cReactionLine data-id=1-0></p></div>


		<div class=cReply id=c1-1><p class=cReplyHeader><em><a href=#c1-1>#c1-1</a> by <span class=cPosterUsername>testuser-guest</span> on 1970-01-01</em></p>
		<p>Hello reply.</p>
		<p class=cReactionLine data-id=1-1></p></div>


		<div class=cReply id=c1-2><p class=cReplyHeader><em><a href=#c1-2>#c1-2</a> by <span class=cPosterUsername>testuser-guest</span> on 1970-01-01</em></p>
		<p>Another reply here.</p>
		<p class=cReactionLine data-id=1-2></p></div>
		<div class='cReply cNeedsJS'><div></div><p><textarea placeholder='Write reply' id=eReplyEditor-1-3 data-id=1-3 rows=1></textarea></p><div></div></div>


		<div class=cComment id=c2><p class=cReplyHeader><em><a href=#c2>#c2</a> by <span class=cPosterUsername>testuser-guest</span> on 1970-01-01</em></p>
		<p>Hello another top comment.</p>
		<p class=cReactionLine data-id=2-0></p></div>
		<div class='cReply cNeedsJS'><div></div><p><textarea placeholder='Write reply' id=eReplyEditor-2-1 data-id=2-1 rows=1></textarea></p><div></div></div>
		<p><b>Add new comment:</b></p><div class='cComment cNeedsJS'><div></div><p><textarea placeholder='Write new top level comment here...' id=eReplyEditor-3-0 data-id=3-0 rows=1></textarea></p><div></div></div><p class=cNoJSNote>(Adding a new comment or reply requires javascript.)</p><p id=eAccountpageLink></p><hr><p><a href=/>to the frontpage</a></p>
		<script>
		  let PostName = 'samplegood'
		  let PostRenderTS = 1900033
		  let ReactionCounts = {
		  }
		  ReactionNotes = {
		  }
		  let Userinfos = {
		    "testuser-guest": "2066-July (-1158 months ago)",
		  }
		  let iioui = null
		  async function iioinit() {
		    let iiomodule = await import("./iio.js")
		    iioui = iiomodule.iioui
		    iiomodule.iio.Run(iioui.Init)
		  }
		  iioinit()
		</script>
		</body></html>
	`,
	)

	efft.Effect(efft.Diff(string(baseRender), string(laterRender))).Equals(`
		 <script>
		   let PostName = 'samplegood'
		-  let PostRenderTS = 1900033
		-  let ReactionCounts = {
		-  }
		-  ReactionNotes = {
		+  let PostRenderTS = 88300043
		+  let ReactionCounts = {
		+    '0-0-dislike': 1,
		+    '1-1-like': 1,
		+  }
		+  ReactionNotes = {
		+    '1-1-like': [ "This is a note!", ],
		   }
		   let Userinfos = {
	`)
}
