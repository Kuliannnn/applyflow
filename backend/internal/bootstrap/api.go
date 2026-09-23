package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"applyflow/backend/internal/adapters/localfiles"
	"applyflow/backend/internal/adapters/postgres"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/auth"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/platform/config"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func RunAPI(ctx context.Context, c config.Config, logger *slog.Logger) error {
	db, err := sql.Open("pgx", c.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(c.DBMaxConns)
	db.SetMaxIdleConns(c.DBMaxConns)
	db.SetConnMaxLifetime(30 * time.Minute)
	var draining atomic.Bool
	ready := func(ctx context.Context) error {
		if draining.Load() {
			return errors.New("server draining")
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := db.PingContext(bounded); err != nil {
			return err
		}
		var version int64
		if err := db.QueryRowContext(bounded, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
			return err
		}
		if version != 6 {
			return errors.New("database schema incompatible; apply migrations separately")
		}
		return nil
	}
	if err = ready(ctx); err != nil {
		return errors.New("database unavailable or schema incompatible; run migrate up first")
	}
	authService, err := auth.New(postgres.AuthStore{DB: db}, security.Passwords{Cost: 12})
	if err != nil {
		return errors.New("password service unavailable")
	}
	blobs, err := localfiles.Open(c.FileStorageDir)
	if err != nil {
		return err
	}
	defer blobs.Close()
	validator, err := localfiles.NewValidator(c.PDFInfoBin)
	if err != nil {
		return errors.New("pdfinfo unavailable; install Poppler or set PDFINFO_BIN")
	}
	gin.SetMode(gin.ReleaseMode)
	handler := httpapi.New(authService, studio.New(postgres.WorkspaceStore{DB: db}), httpapi.Options{Exports: postgres.FlowStore{DB: db}, Intake: &intake.Service{Store: postgres.FlowStore{DB: db}}, Generations: postgres.FlowStore{DB: db}, Documents: postgres.FlowStore{DB: db}, Tasks: postgres.FlowStore{DB: db}, Files: files.New(postgres.FileStore{DB: db}, blobs, validator), Resumes: resume.New(postgres.ResumeStore{DB: db}), Origin: c.PublicOrigin, SessionKey: c.SessionKey, CSRFKey: c.CSRFKey, SessionTTL: c.SessionTTL, Ready: ready, Logger: logger})
	server := &http.Server{Addr: c.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	listener, err := net.Listen("tcp", c.Address)
	if err != nil {
		return errors.New("cannot listen on HTTP_ADDR")
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	logger.Info("API started", "address", listener.Addr().String(), "mode", c.Environment)
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		draining.Store(true)
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
		<-done
		return err
	}
}
