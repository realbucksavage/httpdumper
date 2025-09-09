package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

var (
	port int
	echo bool
)

func init() {
	flag.IntVar(&port, "port", 8080, "Port to listen on")
	flag.BoolVar(&echo, "echo", false, "Echo the request back to the caller")
}

func main() {
	flag.Parse()

	addr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(handle),
		ReadHeaderTimeout: 15 * time.Second,
	}

	log.Printf("httpdumper starting on %s (echo=%v)\n", addr, echo)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func handle(w http.ResponseWriter, r *http.Request) {
	// Capture request information and body for possible echo
	var dump bytes.Buffer
 body, truncated, _ := captureAndDump(&dump, r)

	// Print to stdout
	os.Stdout.Write(dump.Bytes())

	if echo {
		// Echo back request headers (excluding hop-by-hop) and body
		copyEchoHeaders(w.Header(), r.Header)
		// If client provided Content-Type, preserve it; otherwise keep default
		if ct := r.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		} else if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		// If body was truncated due to server cap, reflect that exact bytes; optionally indicate via header
		if truncated {
			w.Header().Set("X-Echo-Note", "body truncated by server cap")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "OK\n")
}

// dumpRequest is kept for compatibility but unused in echo path.
// copyEchoHeaders copies request headers to response headers excluding hop-by-hop headers.
func copyEchoHeaders(dst http.Header, src http.Header) {
	if src == nil || dst == nil { return }
	hop := map[string]struct{}{
		"Connection": {},
		"Keep-Alive": {},
		"Proxy-Connection": {},
		"Transfer-Encoding": {},
		"Upgrade": {},
		"TE": {},
		"Trailer": {},
		"Content-Length": {},
		"Host": {},
	}
	for k, vals := range src {
		if _, skip := hop[k]; skip { continue }
		for _, v := range vals { dst.Add(k, v) }
	}
}

func dumpRequest(w io.Writer, r *http.Request) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	// Start line
	fmt.Fprintf(bw, "----- REQUEST DUMP (%s) -----\n", time.Now().Format(time.RFC3339Nano))
	fmt.Fprintf(bw, "%s %s %s\n", r.Method, r.URL.RequestURI(), r.Proto)

	// Remote info
	fmt.Fprintf(bw, "RemoteAddr: %s\n", r.RemoteAddr)
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		fmt.Fprintf(bw, "RemoteIP: %s\n", ip)
	}
	fmt.Fprintf(bw, "Host: %s\n", r.Host)

	// URL parts
	fmt.Fprintf(bw, "Scheme: %s\n", schemeOf(r))
	fmt.Fprintf(bw, "Path: %s\n", r.URL.Path)
	fmt.Fprintf(bw, "RawQuery: %s\n", r.URL.RawQuery)

	// Headers sorted for stability
	fmt.Fprintln(bw, "Headers:")
	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(bw, "  %s: %s\n", k, strings.Join(r.Header[k], ", "))
	}

	// Cookies
	if cs := r.Cookies(); len(cs) > 0 {
		fmt.Fprintln(bw, "Cookies:")
		for _, c := range cs {
			fmt.Fprintf(bw, "  %s=%s; Path=%s; Domain=%s; HttpOnly=%v; Secure=%v\n", c.Name, c.Value, c.Path, c.Domain, c.HttpOnly, c.Secure)
		}
	}

	// Body (cap to 10MB)
	const maxBody = 10 << 20
	body, bodyErr := readAllWithCap(r.Body, maxBody)
	defer r.Body.Close()
	if bodyErr != nil {
		fmt.Fprintf(bw, "BodyError: %v\n", bodyErr)
	}
	fmt.Fprintf(bw, "BodyBytes: %d\n", len(body))
	if len(body) > 0 {
		fmt.Fprintln(bw, "Body:")
		bw.Write(body)
		if len(body) == maxBody {
			fmt.Fprintln(bw, "\n-- body truncated --")
		} else {
			fmt.Fprintln(bw)
		}
	}

	// Trailers (after body read)
	if len(r.Trailer) > 0 {
		fmt.Fprintln(bw, "Trailers:")
		tkeys := make([]string, 0, len(r.Trailer))
		for k := range r.Trailer {
			tkeys = append(tkeys, k)
		}
		sort.Strings(tkeys)
		for _, k := range tkeys {
			fmt.Fprintf(bw, "  %s: %s\n", k, strings.Join(r.Trailer[k], ", "))
		}
	}

	// TLS info
	if r.TLS != nil {
		fmt.Fprintln(bw, "TLS:")
		fmt.Fprintf(bw, "  Version: %x\n", r.TLS.Version)
		fmt.Fprintf(bw, "  CipherSuite: %x\n", r.TLS.CipherSuite)
		fmt.Fprintf(bw, "  ServerName: %s\n", r.TLS.ServerName)
	}

	fmt.Fprintln(bw, "-------------------------------")
}

// captureAndDump writes a diagnostic dump to w, consumes the request body once,
// and returns the consumed body, whether it was truncated by cap, and any error encountered.
func captureAndDump(w io.Writer, r *http.Request) ([]byte, bool, error) {
	const maxBody = 10 << 20
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	// Start line
	fmt.Fprintf(bw, "----- REQUEST DUMP (%s) -----\n", time.Now().Format(time.RFC3339Nano))
	fmt.Fprintf(bw, "%s %s %s\n", r.Method, r.URL.RequestURI(), r.Proto)

	// Remote info
	fmt.Fprintf(bw, "RemoteAddr: %s\n", r.RemoteAddr)
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		fmt.Fprintf(bw, "RemoteIP: %s\n", ip)
	}
	fmt.Fprintf(bw, "Host: %s\n", r.Host)

	// URL parts
	fmt.Fprintf(bw, "Scheme: %s\n", schemeOf(r))
	fmt.Fprintf(bw, "Path: %s\n", r.URL.Path)
	fmt.Fprintf(bw, "RawQuery: %s\n", r.URL.RawQuery)

	// Headers sorted for stability
	fmt.Fprintln(bw, "Headers:")
	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(bw, "  %s: %s\n", k, strings.Join(r.Header[k], ", "))
	}

	// Cookies
	if cs := r.Cookies(); len(cs) > 0 {
		fmt.Fprintln(bw, "Cookies:")
		for _, c := range cs {
			fmt.Fprintf(bw, "  %s=%s; Path=%s; Domain=%s; HttpOnly=%v; Secure=%v\n", c.Name, c.Value, c.Path, c.Domain, c.HttpOnly, c.Secure)
		}
	}

	// Body (cap to 10MB)
	body, bodyErr := readAllWithCap(r.Body, maxBody)
	defer r.Body.Close()
	if bodyErr != nil {
		fmt.Fprintf(bw, "BodyError: %v\n", bodyErr)
	}
	fmt.Fprintf(bw, "BodyBytes: %d\n", len(body))
	if len(body) > 0 {
		fmt.Fprintln(bw, "Body:")
		bw.Write(body)
		if len(body) == maxBody {
			fmt.Fprintln(bw, "\n-- body truncated --")
		} else {
			fmt.Fprintln(bw)
		}
	}

	// Trailers (after body read)
	if len(r.Trailer) > 0 {
		fmt.Fprintln(bw, "Trailers:")
		tkeys := make([]string, 0, len(r.Trailer))
		for k := range r.Trailer {
			tkeys = append(tkeys, k)
		}
		sort.Strings(tkeys)
		for _, k := range tkeys {
			fmt.Fprintf(bw, "  %s: %s\n", k, strings.Join(r.Trailer[k], ", "))
		}
	}

	// TLS info
	if r.TLS != nil {
		fmt.Fprintln(bw, "TLS:")
		fmt.Fprintf(bw, "  Version: %x\n", r.TLS.Version)
		fmt.Fprintf(bw, "  CipherSuite: %x\n", r.TLS.CipherSuite)
		fmt.Fprintf(bw, "  ServerName: %s\n", r.TLS.ServerName)
	}

 fmt.Fprintln(bw, "-------------------------------")
 return body, len(body) == maxBody, bodyErr
 }

 func readAllWithCap(rc io.ReadCloser, capBytes int) ([]byte, error) {
	if rc == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	buf.Grow(intMin(capBytes, 4096))
	limReader := io.LimitedReader{R: rc, N: int64(capBytes)}
	_, err := buf.ReadFrom(&limReader)
	if err != nil {
		return buf.Bytes(), err
	}
	if limReader.N == 0 { // exactly hit cap, may have more data
		return buf.Bytes(), nil
	}
	return buf.Bytes(), nil
}

func intMin(a, b int) int { if a < b { return a }; return b }

func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	// Honor X-Forwarded-Proto or Forwarded when present
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		return xf
	}
	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		// very simple parse proto= value
		parts := strings.Split(fwd, ";")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(strings.ToLower(p), "proto=") {
				return strings.Trim(strings.SplitN(p, "=", 2)[1], "\"")
			}
		}
	}
	return "http"
}
