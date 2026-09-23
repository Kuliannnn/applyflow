package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"applyflow/backend/internal/bootstrap"
	"applyflow/backend/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "reason", err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = bootstrap.RunAPI(ctx, c, logger); err != nil {
		logger.Error("API stopped", "reason", err.Error())
		os.Exit(1)
	}
}
