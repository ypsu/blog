package main

import (
	"blog/abname"
	"blog/alogdb"
	"blog/email"
	"blog/eventz"
	"blog/posts"
	"blog/sig"
	"blog/userapi"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter

	statuscode int
	firstWrite string
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.firstWrite == "" {
		if len(p) > 256 {
			w.firstWrite = string(p[:256])
		} else {
			w.firstWrite = string(p)
		}
	}
	return w.ResponseWriter.Write(p)
}

func (w *loggingResponseWriter) WriteHeader(statuscode int) {
	w.statuscode = statuscode
	w.ResponseWriter.WriteHeader(statuscode)
}

var recentRequests atomic.Int64
var shedThreshold int64 = 1000
var totalShed int64

func ratelimiter() {
	ez := eventz.Default
	for {
		old := recentRequests.Swap(0)
		if old > shedThreshold {
			totalShed += old - shedThreshold
			ez.Printf("serve.ShedRequests count=%d total=%d", old-shedThreshold, totalShed)
		} else if old > shedThreshold/2 {
			ez.Printf("serve.QPSNearSheddingThreshold count=%d threshold=%d", old, shedThreshold)
		}
		time.Sleep(2 * time.Second)
	}
}

func handleFunc(w http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/wp-") || strings.HasSuffix(req.URL.Path, ".php") {
		// Reject wordpress scanner spam right away.
		http.Error(w, "serve.NoWordpressHere", http.StatusNotFound)
		return
	}
	if recentRequests.Add(1) > shedThreshold {
		http.Error(w, "serve.ServerOverloaded (receiving too many requests, come back a few hours later)", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	if req.Host == "iio.ie" || req.Host == "www.iio.ie" {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000")
	}

	if strings.HasPrefix(req.Host, "www.") {
		target := "https://" + req.Host[4:] + req.URL.String()
		http.Redirect(w, req, target, http.StatusMovedPermanently)
		return
	}

	lw := &loggingResponseWriter{ResponseWriter: w}
	user := userapi.DefaultDB.Username(w, req)
	switch {
	case req.URL.Path == "/msgauthwait":
		email.HandleMsgauthwait(lw, req)
	case req.URL.Path == "/sig":
		sig.HandleHTTP(lw, req)
	case req.URL.Path == "/userapi":
		userapi.DefaultDB.HandleHTTP(lw, req)
	case req.URL.Path == "/eventz" && user == "iio":
		eventz.Default.ServeHTTP(lw, req)
	default:
		posts.HandleHTTP(lw, req)
	}

	errstr := ""
	if lw.statuscode >= 400 {
		if lw.firstWrite == "posts.PostNotFound\n" {
			// Ignore spam for now.
			return
		}
		errstr = fmt.Sprintf(" err=%q", lw.firstWrite)
	}
	log.Printf("serve.Request method=%s path=%q statuscode=%d dur=%0.3fms%s referer=%q agent=%q", req.Method, req.URL, lw.statuscode, float64(time.Since(start).Microseconds())/1000.0, errstr, req.Header.Get("Referer"), req.Header.Get("User-Agent"))
}

func run(ctx context.Context) error {
	flagAPI := flag.String("api", "http://localhost:8787", "The address of the cloudflare backend.")
	flagAddress := flag.String("address", ":8080", "The listening address for the server.")
	flagAlogdb := flag.String("alogdb", "", "The local file to use as the alogdb backend for testing purposes. Empty means using the production table.")
	syscall.Mlockall(7) // never swap data to disk.
	log.SetFlags(log.Flags() | log.Lmicroseconds | log.Lshortfile)
	flag.Parse()

	if err := abname.Init(); err != nil {
		return fmt.Errorf("server.AbnameInit: %v", err)
	}

	var db *alogdb.DB
	var err error
	if *flagAlogdb != "" {
		fh, err := os.OpenFile(*flagAlogdb, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("serve.OpenAlogdb: %v", err)
		}
		db, err = alogdb.NewForTesting(fh)
		if err != nil {
			return fmt.Errorf("serve.NewTestingAlogdb: %v", err)
		}
	} else {
		db, err = alogdb.New(ctx, *flagAPI)
		if err != nil {
			return fmt.Errorf("serve.NewAlogdb: %v", err)
		}
	}
	alogdb.DefaultDB = db

	userapi.DefaultDB.Init()
	posts.APIAddress = *flagAPI
	posts.Init()
	posts.LoadPosts()
	go ratelimiter()

	http.HandleFunc("/", handleFunc)
	server := &http.Server{
		Addr:        *flagAddress,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errch := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			errch <- err
		}
	}()

	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			posts.LoadPosts()
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errch:
		return fmt.Errorf("server.ListenAndServe: %v", err)
	}

	log.Printf("waiting for the requests to finish.")
	if err := server.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("server.Shutdown(): %v", err)
	}
	log.Print("server.Shutdown() succeeded")
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGQUIT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}
