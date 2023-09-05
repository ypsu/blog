package main

import (
	"blog/email"
	"blog/posts"
	"blog/sig"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func handleFunc(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL)

	if strings.HasPrefix(req.Host, "www.") {
		target := "https://" + req.Host[4:] + req.URL.String()
		http.Redirect(w, req, target, http.StatusMovedPermanently)
		return
	}

	if req.Host == "iio.ie" {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000")
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
	s := <-sigquits
	log.Printf("%s signal received, exiting.", s)
}
