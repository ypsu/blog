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
	"syscall"
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

	if isadmin := userapi.DefaultDB.Username(w, req) == "iio"; isadmin {
		switch req.URL.Path {
		case "/eventz":
			eventz.Default.ServeHTTP(w, req)
			return
		}
	}

	switch req.URL.Path {
	case "/msgauthwait":
		email.HandleMsgauthwait(w, req)
	case "/sig":
		sig.HandleHTTP(w, req)
	case "/userapi":
		userapi.DefaultDB.HandleHTTP(w, req)
	default:
		posts.HandleHTTP(w, req)
	}
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
