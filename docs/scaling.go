package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var url = flag.String("url", "", "url to loadtest")

var stats struct {
	finished atomic.Int64
}

func query() error {
	resp, err := http.Get(*url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got status %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	stats.finished.Add(1)
	return nil
}

func run() error {
	http.DefaultTransport.(*http.Transport).DisableKeepAlives = true
	flag.Parse()
	if *url == "" {
		return errors.New("missing --url flag")
	}

	fmt.Println("the finished number is the number of requests finished in the last second.")
	fmt.Println("whatever max number you see there is the max qps the target server can take.")
	ticker := time.NewTicker(time.Second)
	var started, lastfin int64
	for it := 2; ; it++ {
		ticker.Reset(1e9 * time.Nanosecond / time.Duration(it))
		for i := 0; i < it; i++ {
			<-ticker.C
			started++
			go func() {
				if err := query(); err != nil {
					log.Fatal(err)
				}
			}()
		}
		newfin := stats.finished.Load()
		fmt.Printf("stats for the last second: active:%d finished:%d\n", started-newfin, newfin-lastfin)
		lastfin = newfin
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
