// Command migrate applies embedded SQL migrations. It intentionally exposes no down command.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"applyflow/backend/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv("DATABASE_URL")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dsn string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return errors.New("usage: migrate up|status (DATABASE_URL required)")
	}
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("invalid database configuration (connection string omitted)")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	err = db.PingContext(pingCtx)
	pingCancel()
	if err != nil {
		return errors.New("database connection failed (connection details omitted)")
	}

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("configure migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if args[0] == "up" {
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		fmt.Printf("Applied %d migration(s).\n", len(results))
		return nil
	}
	statuses, err := provider.Status(ctx)
	if err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	for _, status := range statuses {
		fmt.Printf("%s\t%s\n", status.Source.Path, status.State)
	}
	return nil
}
