package main

import (
	"blog/abname"
	"blog/email"
	"blog/msgz"
	"blog/posts"
	"blog/sig"
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

	if req.URL.Path == "/msgz" {
		msgz.Default.HandleMsgZ(w, req)
		return
	}

	switch req.URL.Path {
	case "/msgauthwait":
		email.HandleMsgauthwait(w, req)
	case "/sig":
		sig.HandleHTTP(w, req)
	default:
		posts.HandleHTTP(w, req)
	}
}

func run(ctx context.Context) error {
	flagAddress := flag.String("address", ":8080", "The listening address for the server.")
	syscall.Mlockall(7) // never swap data to disk.
	log.SetFlags(log.Flags() | log.Lmicroseconds | log.Lshortfile)
	flag.Parse()

	if err := abname.Init(); err != nil {
		return fmt.Errorf("server.AbnameInit: %v", err)
	}

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

	<-ctx.Done()
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
