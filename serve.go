package main

import (
	"flag"
	"log"
	"net/http"
	"notech/monitoring"
	"notech/posts"
	"notech/server"
	"notech/sig"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var acmepathFlag = flag.String("acmepath", "", "the directory for the acme challenge.")
var dumpallFlag = flag.Bool("dumpall", false, "if true dumps the backup page to stdout.")

var acmehandler http.Handler

func handleFunc(w http.ResponseWriter, req *http.Request) {
	if acmehandler != nil && strings.HasPrefix(req.URL.Path, "/.well-known/acme-challenge/") {
		acmehandler.ServeHTTP(w, req)
	}

	if req.Host == "www.notech.ie" {
		target := "https://" + req.Host[4:] + req.URL.String()
		http.Redirect(w, req, target, http.StatusMovedPermanently)
		return
	}

	if req.TLS == nil {
		if req.Host == "notech.ie" {
			target := "https://notech.ie" + req.URL.String()
			http.Redirect(w, req, target, http.StatusMovedPermanently)
			return
		}
	} else {
		if req.Host == "notech.ie" {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000")
		}
	}

	if req.URL.Path == "/monitoringprobe" {
		monitoring.HandleProber(w, req)
		return
	}

	if req.URL.Path == "/sig" {
		sig.HandleHTTP(w, req)
		return
	}

	posts.HandleHTTP(w, req)
}

func main() {
	syscall.Mlockall(7) // never swap data to disk.
	flag.Parse()

	server.LoadCert()
	posts.LoadPosts()
	if *dumpallFlag {
		posts.DumpAll(os.Stdout)
		return
	}

	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			server.LoadCert()
			posts.LoadPosts()
		}
	}()

	server.Init()
	server.ServeMux.HandleFunc("/", handleFunc)
	if len(*acmepathFlag) > 0 {
		acmehandler = http.FileServer(http.Dir(*acmepathFlag))
	}
	sig.Init()
	monitoring.Init()

	sigquits := make(chan os.Signal, 2)
	signal.Notify(sigquits, syscall.SIGQUIT)
	<-sigquits
	log.Print("sigquit received, exiting")
}
