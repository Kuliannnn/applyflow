package bootstrap

import (
	"applyflow/backend/internal/adapters/localfiles"
	"applyflow/backend/internal/adapters/postgres"
	"applyflow/backend/internal/adapters/provider"
	"applyflow/backend/internal/adapters/render"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/platform/config"
	"applyflow/backend/internal/task"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

func RunWorker(ctx context.Context, c config.Worker, logger *slog.Logger) error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL required")
	}
	db, err := sql.Open("pgx", c.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var version int
	if err = db.QueryRowContext(bounded, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 6 {
		return errors.New("database unavailable or incompatible; run migrations first")
	}
	blobs, err := localfiles.Open(c.FileStorageDir)
	if err != nil {
		return err
	}
	defer blobs.Close()
	renderer, err := render.New(ctx, c.Python)
	if err != nil {
		return err
	}
	store := postgres.FlowStore{DB: db}
	gen := generation.Executor{Store: store, Provider: provider.Mock{}}
	exports := export.Executor{Store: store, Renderer: renderer, Blobs: blobs}
	logger.Info("worker started", "delivery", "postgres-outbox", "generation", "mock-v1", "export_template", "1")
	return (task.Worker{Store: store, Executor: task.Router{"extract_jd": gen, "tailor_resume": gen, "write_cover_letter": gen, "export_document": exports}, Logger: logger}).Run(ctx)
}
