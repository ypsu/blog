package main

import (
	"flag"
	"log"
	"net/http"
	"notech/email"
	"notech/posts"
	"notech/sig"
	"os"
	"os/signal"
	"syscall"
)

func handleFunc(w http.ResponseWriter, req *http.Request) {
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

	if req.URL.Path == "/sig" {
		sig.HandleHTTP(w, req)
		return
	}

	if req.URL.Path == "/msgauthwait" {
		email.HandleMsgauthwait(w, req)
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

	http.HandleFunc("/", handleFunc)
	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			posts.LoadPosts()
		}
	}()

	sigquits := make(chan os.Signal, 2)
	signal.Notify(sigquits, syscall.SIGQUIT, syscall.SIGTERM)
	<-sigquits
	log.Print("sigquit received, exiting")
}
