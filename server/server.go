// package server starts web server for handling gopher, http, https data.
// all connections go through the filter() function
// which limits the number of simultaneous connections from a single ip address.
package server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"
)

// ServeMux can be used to register handlers into the server.
var ServeMux = &http.ServeMux{}

var gopherPort = flag.Int("gopherport", 8070, "port for the gopher service. -1 to disable gopher serving.")
var httpPort = flag.Int("httpport", 8080, "port for the http service. -1 to disable http serving. negative port number to redirect to https.")
var httpsPort = flag.Int("httpsport", 8443, "port for the https service. -1 to disable https serving.")

type listener struct {
	all, filtered chan net.Conn
}

func (l listener) Accept() (net.Conn, error) {
	return <-l.filtered, nil
}

func (listener) Close() error {
	log.Print("called the unimplemented listenerBase.Close")
	return errors.New("unimplemented Close")
}

func (listener) Addr() net.Addr {
	log.Print("called the unimplemented listenerBase.Addr")
	return &net.IPAddr{}
}

type wrappedConn struct {
	*net.TCPConn
	host string
}

var closer = make(chan wrappedConn, 4)

func (c wrappedConn) Close() error {
	closer <- c
	return nil
}

func filter(httpl, httpsl, gopherl *listener) {
	// addrConns counts the number of active connections per ip address.
	addrConns := map[string]int{}
	for {
		var kind string
		var rawconn net.Conn
		var dst chan net.Conn
		select {
		case conn := <-closer:
			cnt := addrConns[conn.host]
			cnt--
			if cnt == 0 {
				delete(addrConns, conn.host)
			} else {
				addrConns[conn.host] = cnt
			}
			if err := conn.TCPConn.Close(); err != nil {
				log.Print(err)
			}
			continue
		case rawconn = <-httpl.all:
			kind = "http"
			dst = httpl.filtered
		case rawconn = <-httpsl.all:
			kind = "https"
			dst = httpsl.filtered
		case rawconn = <-gopherl.all:
			kind = "gopher"
			dst = gopherl.filtered
		}
		conn := wrappedConn{TCPConn: rawconn.(*net.TCPConn)}
		host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			log.Fatal(err)
		}
		cnt := addrConns[host]
		if cnt >= 50 {
			log.Printf("dropped %s connection from %s", kind, rawconn.RemoteAddr().String())
			rawconn.Write([]byte("HTTP/1.0 503 service unavailable (overloaded)\nContent-Length: 18\n\nserver overloaded\n"))
			rawconn.Close()
			continue
		}
		if len(addrConns) > 900 {
			log.Printf("dropped %s connection from %s", kind, rawconn.RemoteAddr().String())
			rawconn.Write([]byte("HTTP/1.0 503 service unavailable (overloaded)\nContent-Length: 18\n\nserver very overloaded\n"))
			rawconn.Close()
			continue
		}
		cnt++
		addrConns[host] = cnt
		conn.host = host
		dst <- conn
	}
}

func foreverAccept(port int) chan net.Conn {
	ch := make(chan net.Conn)
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				log.Fatal(err)
			}
			ch <- c
		}
	}()
	return ch
}

type gopherResponse struct {
	hdr http.Header
	buf bytes.Buffer
}

func (r *gopherResponse) Header() http.Header           { return r.hdr }
func (*gopherResponse) WriteHeader(int)                 {}
func (r *gopherResponse) Write(buf []byte) (int, error) { return r.buf.Write(buf) }

func handleGopher(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	line, err := r.ReadString('\n')
	if err != nil {
		log.Printf("gopher handler error: %v", err)
		return
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 {
		line = "/"
	}
	u, err := url.Parse(line)
	if err != nil {
		log.Printf("gopher url error: %v", err)
		return
	}
	req := &http.Request{URL: u, Proto: "gopher"}
	rw := &gopherResponse{hdr: http.Header{}}
	ServeMux.ServeHTTP(rw, req)
	conn.Write(rw.buf.Bytes())
}

func gopherServer(l *listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleGopher(conn)
	}
}

var cert *tls.Certificate
var certpath = flag.String("certpath", "/home/rlblaster/.d/certbot/live/notech.ie/", "path to the certificates.")

func loadCert() {
	log.Print("(re)loading tls certs from ", *certpath)
	newcert, err := tls.LoadX509KeyPair(
		path.Join(*certpath, "fullchain.pem"),
		path.Join(*certpath, "privkey.pem"))
	if err != nil {
		log.Fatal(err)
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&cert)), (unsafe.Pointer(&newcert)))
}

func getCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return (*tls.Certificate)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&cert)))), nil
}

func redirect(w http.ResponseWriter, req *http.Request) {
	target := "https://" + req.Host + req.URL.String()
	if *httpsPort != 443 {
		host, _, _ := net.SplitHostPort(req.Host)
		target = fmt.Sprintf("https://%s:%d%s", host, *httpsPort, req.URL.String())
	}
	http.Redirect(w, req, target, http.StatusMovedPermanently)
}

// Init starts the server in the background.
func Init() {
	// open listeners and filter them.
	var gopherListener, httpListener, httpsListener listener
	if *gopherPort != -1 {
		gopherListener = listener{foreverAccept(*gopherPort), make(chan net.Conn, 4)}
	}
	if *httpPort < -1 {
		go func() {
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", -*httpPort), http.HandlerFunc(redirect)))
		}()
	} else if *httpPort != -1 {
		httpListener = listener{foreverAccept(*httpPort), make(chan net.Conn, 4)}
	}
	if *httpsPort != -1 {
		httpsListener = listener{foreverAccept(*httpsPort), make(chan net.Conn, 4)}
	}
	go filter(&httpListener, &httpsListener, &gopherListener)
	server := &http.Server{}
	server.Handler = ServeMux
	server.ReadHeaderTimeout = 3 * time.Second
	server.IdleTimeout = 5 * time.Second
	server.ReadTimeout = 30 * time.Minute
	server.WriteTimeout = 30 * time.Minute

	// set up safe tls per https://blog.gopheracademy.com/advent-2016/exposing-go-on-the-internet/.
	server.TLSConfig = &tls.Config{
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	// load certs and keep reloading it on sigint.
	loadCert()
	server.TLSConfig = &tls.Config{GetCertificate: getCert}
	sigints := make(chan os.Signal, 2)
	signal.Notify(sigints, os.Interrupt)
	go func() {
		for range sigints {
			loadCert()
		}
	}()

	// start the servers.
	if gopherListener.all != nil {
		go func() { gopherServer(&gopherListener) }()
	}
	if httpListener.all != nil {
		go func() { log.Print(server.Serve(httpListener)) }()
	}
	if httpsListener.all != nil {
		go func() { log.Print(server.ServeTLS(httpsListener, "", "")) }()
	}
	log.Print("server started")
}
