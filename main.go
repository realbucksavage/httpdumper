package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	port           int
	uiPort         int
	echo           bool
	historySize    int
	shutdownTOFlag time.Duration
)

func init() {
	flag.IntVar(&port, "port", 8080, "Port to listen on for request handling")
	flag.IntVar(&uiPort, "ui-port", 0, "Port to serve the UI (/ui, /requests.json). If 0, UI is served on the main port for backward compatibility")
	flag.BoolVar(&echo, "echo", false, "Echo the request back to the caller")
	flag.IntVar(&historySize, "history", 1000, "Number of recent requests to keep in memory for the UI")
	flag.DurationVar(&shutdownTOFlag, "shutdown-timeout", 10*time.Second, "Graceful shutdown timeout (e.g., 10s, 1m)")
}

func main() {
	flag.Parse()
	if historySize <= 0 {
		historySize = 1000
	}

	// Main server (request handling)
	addr := fmt.Sprintf(":%d", port)
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/", handle)

	mainServer := &http.Server{
		Addr:              addr,
		Handler:           mainMux,
		ReadHeaderTimeout: 15 * time.Second,
	}

	log.Printf("httpdumper main server starting on %s (echo=%v, history=%d)\n", addr, echo, historySize)

	// Channel to receive server errors
	errCh := make(chan error, 2)

	// Optional UI server pointer for shutdown
	var uiServer *http.Server

	// If uiPort > 0, start a separate UI server
	if uiPort > 0 {
		uiAddr := fmt.Sprintf(":%d", uiPort)
		uimux := http.NewServeMux()
		uimux.HandleFunc("/", uiHandler)
		uimux.HandleFunc("/requests.json", jsonHandler)
		uiServer = &http.Server{
			Addr:              uiAddr,
			Handler:           uimux,
			ReadHeaderTimeout: 15 * time.Second,
		}
		go func() {
			log.Printf("httpdumper UI server starting on %s\n", uiAddr)
			err := uiServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("UI server error: %w", err)
			} else {
				errCh <- nil
			}
		}()
	}

	// Start main server in goroutine to enable signal handling
	go func() {
		err := mainServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("main server error: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("signal received: %v; initiating graceful shutdown (timeout=%s)", sig, shutdownTOFlag)
	case err := <-errCh:
		if err != nil {
			log.Printf("server error before shutdown: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTOFlag)
	defer cancel()

	// Shutdown servers
	if uiServer != nil {
		if err := uiServer.Shutdown(ctx); err != nil {
			log.Printf("UI server shutdown error: %v", err)
		}
	}
	if err := mainServer.Shutdown(ctx); err != nil {
		log.Printf("Main server shutdown error: %v", err)
	}

	log.Printf("shutdown complete")
}
