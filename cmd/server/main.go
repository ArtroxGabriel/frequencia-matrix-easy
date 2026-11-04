package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"frequencia-matrix-integration/internal/matrix"
	"frequencia-matrix-integration/internal/store"
	"frequencia-matrix-integration/internal/web"
)

func main() {
	cfg := LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0755); err != nil {
		log.Fatalf("failed creating database directory: %v", err)
	}

	s, err := store.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed initializing database: %v", err)
	}
	defer func() { _ = s.Close() }()

	gw, err := createMatrixGateway(cfg)
	if err != nil {
		log.Fatalf("failed initializing matrix gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startAutoJoin(ctx, gw, cfg.AutoJoinServers)

	handler, err := web.NewHandler(s, gw)
	if err != nil {
		log.Fatalf("failed creating web handler: %v", err)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go runServer(server, cfg.Port, cfg.DevMode)
	waitForShutdown(server)
}

func createMatrixGateway(cfg Config) (matrix.MatrixGateway, error) {
	if cfg.DevMode {
		log.Println("[INFO] running in DEV_MODE with FakeMatrixClient")
		return matrix.NewFakeMatrixClient(true), nil
	}
	return matrix.NewMautrixGateway(cfg.HomeserverURL, cfg.UserID, cfg.AccessToken)
}

// startAutoJoin makes the bot accept room invites in the background, when the
// gateway supports it (the development fake does not).
func startAutoJoin(ctx context.Context, gw matrix.MatrixGateway, allowedServers []string) {
	acceptor, ok := gw.(matrix.InviteAcceptor)
	if !ok {
		return
	}
	if len(allowedServers) == 0 {
		log.Println("[WARN] MATRIX_AUTOJOIN_SERVERS is empty: the bot accepts invites from any homeserver")
	} else {
		log.Printf("[INFO] the bot accepts invites only from: %v", allowedServers)
	}
	go func() {
		if err := acceptor.RunAutoJoin(ctx, allowedServers); err != nil {
			log.Printf("[ERROR] accepting room invites stopped: %v", err)
		}
	}()
}

func runServer(server *http.Server, port int, devMode bool) {
	log.Printf("[INFO] Server listening on port :%d (devMode=%v)", port, devMode)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server listen error: %v", err)
	}
}

func waitForShutdown(server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[INFO] shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] server forced to shutdown: %v", err)
	}
	log.Println("[INFO] server stopped")
}
