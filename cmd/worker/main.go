package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	// feature:observability:start
	"github.com/rock288/go-mongo-boilerplate/internal/platform/observability"
	// feature:observability:end
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// feature:observability:start
	observability.BuildVersion = version
	observability.BuildCommit = commit
	// feature:observability:end

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.yaml"
	}

	app, cleanup, err := InitializeWorker(cfgPath)
	if err != nil {
		log.Fatalf("init worker: %v", err)
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app.Logger.Info("worker starting",
		// feature:kafka:start
		slog.String("group", app.Config.Kafka.GroupID),
		// feature:kafka:end
		slog.String("version", version),
		slog.String("commit", commit),
	)

	if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		app.Logger.Error("worker stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
	app.Logger.Info("worker exited cleanly")
}
