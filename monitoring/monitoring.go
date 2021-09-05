// package monitoring does some periodic self-health checks and other routines.
package monitoring

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"time"
)

var (
	dnsurl    = flag.String("dnsurl", "", "this url will be periodically fetched. use it to maintain dns records.")
	mondomain = flag.String("mondomain", "", "the domain to monitor if any.")
	serverid  = fmt.Sprintf("%d", time.Now().UnixNano())
)

// Init starts a background goroutine to monitor the server health.
// it regularly updates dns records check and
// whether a domain is serving this server if requested.
func Init() {
	go func() {
		for {
			checkhealth()
			time.Sleep(30 * time.Second)
		}
	}()
}

func HandleProber(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte(serverid))
}

func checkhealth() {
	client := &http.Client{Timeout: 10 * time.Second}

	if len(*dnsurl) > 0 {
		_, err := client.Get(*dnsurl)
		if err != nil {
			Alert(fmt.Sprintf("can't refresh dns: %v", err))
			return
		}
	}

	if len(*mondomain) > 0 {
		r, err := client.Get("https://" + *mondomain + "/monitoringprobe")
		if err != nil {
			Alert(fmt.Sprintf("can't fetch the site: ", err))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			Alert(fmt.Sprintf("can't read body of the site: ", err))
			return
		}
		if bytes.Compare(body, []byte(serverid)) != 0 {
			if len(body) > 100 {
				body = body[:100]
			}
			Alert(fmt.Sprintf("unexpected body: %q", body))
			return
		}
	}

	client.CloseIdleConnections()
}

var alertfile = path.Join(os.Getenv("HOME"), "todo")

// Alert puts a message into the todo file if it doesn't already contain it.
func Alert(msg string) {
	log.Printf("alert: %s", msg)
	fmt.Print("\a")
	contents, err := os.ReadFile(alertfile)
	if err != nil {
		log.Fatal(err)
	}
	if bytes.Contains(contents, []byte("#notechalert ")) {
		return
	}
	f, err := os.OpenFile(alertfile, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	fmt.Fprintf(f, "\n#notechalert %s\n", msg)
}
