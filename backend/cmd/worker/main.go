package main

import (
	"applyflow/backend/internal/bootstrap"
	"applyflow/backend/internal/platform/config"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c, err := config.LoadWorker(os.Getenv)
	if err != nil {
		logger.Error("invalid worker configuration", "reason", err.Error())
		os.Exit(1)
	}
	if err := bootstrap.RunWorker(ctx, c, logger); err != nil {
		logger.Error("worker stopped", "reason", err.Error())
		os.Exit(1)
	}
}
