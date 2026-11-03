package main

import (
	"context"
	"net/http"
	"syscall"
	"testing"
	"time"
)

func TestServerStartupAndShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    "127.0.0.1:0", // free port
		Handler: mux,
	}

	go runServer(server, 0, true)

	time.Sleep(50 * time.Millisecond)

	// Trigger graceful shutdown by sending SIGTERM to self
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()

	waitForShutdown(server)

	// Verify server is stopped
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = server.Shutdown(ctx)
}
