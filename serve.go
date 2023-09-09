package main

import (
	"blog/email"
	"blog/posts"
	"blog/sig"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func handleFunc(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL)

	if req.Host == "iio.ie" || req.Host == "www.iio.ie" {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000")
	}

	if strings.HasPrefix(req.Host, "www.") {
		target := "https://" + req.Host[4:] + req.URL.String()
		http.Redirect(w, req, target, http.StatusMovedPermanently)
		return
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

func run() error {
	syscall.Mlockall(7) // never swap data to disk.
	log.SetFlags(log.Flags() | log.Lmicroseconds | log.Lshortfile)
	flag.Parse()

	posts.Init()
	posts.LoadPosts()
	if *posts.DumpallFlag {
		posts.DumpAll()
		return nil
	}

	http.HandleFunc("/", handleFunc)
	server := &http.Server{
		Addr: ":8080",
	}
	go func() {
		err := server.ListenAndServe()
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

	log.Printf("waiting for the requests to finish.", s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server.Shutdown(): %v", err)
	}
	log.Print("server.Shutdown() succeeded")
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
