package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"notech/testwriter"
	"sync"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	_, outputs := testwriter.Data(t)
	for k := range outputs {
		delete(outputs, k)
	}

	ServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected query: %s", r.URL.Path)
	})
	ServeMux.HandleFunc("/sleeper", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("hello world"))
	})
	Init()
	time.Sleep(time.Second)

	// test the gopher server.
	conn, err := net.Dial("tcp", "localhost:8070")
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("/sleeper\r\n"))
	contents, err := io.ReadAll(conn)
	outputs["gopher"] = string(contents)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// test the read header timeout functionality.
	conn, err = net.Dial("tcp", "localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	readstart := time.Now()
	n, err := conn.Read(buf)
	readfinish := time.Now()
	readseconds := readfinish.Sub(readstart) / time.Second
	outputs["norequest"] = fmt.Sprintf("err:%v n:%d response:%q duration:%ds", err, n, buf[:n], readseconds)
	conn.Close()

	// test parallel request limiting.
	cnt := 60
	okch := make(chan struct{}, cnt)
	badch := make(chan struct{}, cnt)
	wg := &sync.WaitGroup{}
	wg.Add(cnt)
	for i := 0; i < cnt; i++ {
		go func() {
			resp, err := http.DefaultClient.Get("http://localhost:8080/sleeper")
			if err == nil && resp.StatusCode == 200 {
				okch <- struct{}{}
			} else {
				badch <- struct{}{}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	close(okch)
	close(badch)
	okcnt, badcnt := 0, 0
	for range okch {
		okcnt++
	}
	for range badch {
		badcnt++
	}
	outputs["sleeper"] = fmt.Sprintf("ok:%d bad:%d", okcnt, badcnt)
}
