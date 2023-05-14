package main

import (
	"flag"
	"log"
	"net/http"
	"notech/email"
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

	posts.Init()
	posts.LoadPosts()
	if *posts.DumpallFlag {
		posts.DumpAll()
		return
	}

	server.LoadCert()
	server.EmailHandler = email.EmailHandler
	server.Init()
	server.ServeMux.HandleFunc("/", handleFunc)
	if len(*acmepathFlag) > 0 {
		acmehandler = http.FileServer(http.Dir(*acmepathFlag))
	}
	monitoring.Init()

	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			server.LoadCert()
			posts.LoadPosts()
		}
	}()

	sigquits := make(chan os.Signal, 2)
	signal.Notify(sigquits, syscall.SIGQUIT)
	<-sigquits
	log.Print("sigquit received, exiting")
}
