package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func handle(w http.ResponseWriter, r *http.Request) {
	// Capture request information and body for possible echo
	var dump bytes.Buffer
	body, truncated, _ := captureAndDump(&dump, r)

	// Print to stdout
	os.Stdout.Write(dump.Bytes())

	// Record for UI
	rec := requestRec{
		Time:       time.Now().Format(time.RFC3339Nano),
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Proto:      r.Proto,
		RemoteAddr: r.RemoteAddr,
		Host:       r.Host,
		Headers:    r.Header.Clone(),
		Body:       string(body),
		BodyBytes:  len(body),
		Truncated:  truncated,
	}
	appendHistory(rec)

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

// copyEchoHeaders copies request headers to response headers excluding hop-by-hop headers.
func copyEchoHeaders(dst http.Header, src http.Header) {
	if src == nil || dst == nil {
		return
	}
	hop := map[string]struct{}{
		"Connection":        {},
		"Keep-Alive":        {},
		"Proxy-Connection":  {},
		"Transfer-Encoding": {},
		"Upgrade":           {},
		"TE":                {},
		"Trailer":           {},
		"Content-Length":    {},
		"Host":              {},
	}
	for k, vals := range src {
		if _, skip := hop[k]; skip {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

// uiHandler serves the embedded static HTML which fetches data from /requests.json.
func uiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if len(uiHTML) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("UI not available"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(uiHTML)
}

// jsonHandler provides the recent requests as JSON (newest first).
func jsonHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	list := snapshotHistory()
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(list)
}
