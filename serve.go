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
	"syscall"
)

var acmepathFlag = flag.String("acmepath", "", "the directory for the acme challenge.")
var dumpallFlag = flag.Bool("dumpall", false, "if true dumps the backup page to stdout.")

func main() {
	flag.Parse()

	posts.Init()
	if *dumpallFlag {
		posts.DumpAll(os.Stdout)
		return
	}

	server.Init()
	server.ServeMux.HandleFunc("/monitoringprobe", monitoring.HandleProber)
	server.ServeMux.HandleFunc("/", posts.HandleHTTP)
	server.ServeMux.HandleFunc("/sig", sig.HandleHTTP)
	if len(*acmepathFlag) > 0 {
		server.ServeMux.Handle("/.well-known/acme-challenge/", http.FileServer(http.Dir(*acmepathFlag)))
	}
	sig.Init()
	monitoring.Init()

	sigquits := make(chan os.Signal, 2)
	signal.Notify(sigquits, syscall.SIGQUIT)
	<-sigquits
	log.Print("sigquit received, exiting")
}
