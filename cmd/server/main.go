package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	observability.BuildVersion = version
	observability.BuildCommit = commit

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.yaml"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, cleanup, err := InitializeServer(cfgPath)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}
	defer cleanup()

	app.Health.Start(ctx)

	app.Logger.Info("server starting",
		slog.Int("port", app.Config.Server.Port),
		slog.String("version", version),
		slog.String("commit", commit),
		slog.Any("observability", app.Config.Observability),
	)

	serverErr := make(chan error, 1)
	go func() {
		if err := app.Server.Start(); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		app.Logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			app.Logger.Error("server failed", slog.String("error", err.Error()))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), app.Config.Server.ShutdownTimeout+10*time.Second)
	defer cancel()
	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		app.Logger.Error("server shutdown failed", slog.String("error", err.Error()))
	}
	// OTel shutdown runs last via cleanup() to flush spans created during drain.
}

// runHealthcheck performs a GET /readyz against the local server. Used as
// the docker HEALTHCHECK in distroless images where curl/wget are absent.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("APP_SERVER__PORT")
	}
	if port == "" {
		port = "8002"
	}
	if _, err := strconv.Atoi(port); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: invalid PORT")
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
