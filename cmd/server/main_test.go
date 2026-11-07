package main

import (
	"context"
	"net/http"
	"syscall"
	"testing"
	"time"

	"frequencia-matrix-integration/internal/matrix"
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

type recordingAcceptor struct {
	matrix.MatrixGateway
	servers chan []string
}

func (r recordingAcceptor) RunAutoJoin(_ context.Context, servers []string) error {
	r.servers <- servers
	return nil
}

func TestStartAutoJoinRunsAcceptorsOnly(t *testing.T) {
	ctx := context.Background()

	startAutoJoin(ctx, matrix.NewFakeMatrixClient(true), nil) // the fake cannot join: no-op

	for _, servers := range [][]string{nil, {"ufc.br"}} {
		acceptor := recordingAcceptor{servers: make(chan []string, 1)}
		startAutoJoin(ctx, acceptor, servers)
		select {
		case got := <-acceptor.servers:
			if len(got) != len(servers) {
				t.Errorf("servers = %v, want %v", got, servers)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("RunAutoJoin was not started")
		}
	}
}
