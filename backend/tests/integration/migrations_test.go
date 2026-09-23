package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"applyflow/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	alice    = "20000000-0000-4000-8000-000000000001"
	bob      = "20000000-0000-4000-8000-000000000002"
	job      = "30000000-0000-4000-8000-000000000001"
	goSkill  = "10000000-0000-4000-8000-000000000001"
	testHash = "$2a$12$....................................................."
)

func isolatedDatabase(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL unset; run make test-migrations-local or make test-integration")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL (redacted)")
	}
	if !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("test database name must end in _test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	admin := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.ExecContext(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	cfg.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

func exec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func scalar(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantSQLState(t *testing.T, ctx context.Context, db *sql.DB, code, query string, args ...any) {
	t.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("want SQLSTATE %s, got %v", code, err)
	}
}

func TestMigrations(t *testing.T) {
	db, ctx := isolatedDatabase(t)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 6 {
		t.Fatalf("expected 6 migrations, got %d", len(results))
	}
	results, err = provider.Up(ctx)
	if err != nil || len(results) != 0 {
		t.Fatalf("second up must be a no-op: %v", err)
	}

	t.Run("reference data and aliases", func(t *testing.T) {
		if n := scalar(t, ctx, db, "SELECT count(DISTINCT skill_id) FROM skill_aliases WHERE alias IN ('go','golang')"); n != 1 {
			t.Fatal(n)
		}
		if n := scalar(t, ctx, db, "SELECT count(*) FROM skills"); n != 8 {
			t.Fatal(n)
		}
	})
	exec(t, ctx, db, "INSERT INTO users(id,email,password_hash) VALUES ($1,'alice@example.com',$3),($2,'bob@example.com',$3)", alice, bob, testHash)
	t.Run("identity and profile constraints", func(t *testing.T) {
		wantSQLState(t, ctx, db, "23505", "INSERT INTO users(email,password_hash) VALUES ('alice@example.com',$1)", testHash)
		wantSQLState(t, ctx, db, "23514", "INSERT INTO users(email,password_hash) VALUES ('Alice@example.com',$1)", testHash)
		wantSQLState(t, ctx, db, "23514", "INSERT INTO users(email,password_hash) VALUES ('eve@example.com','plaintext')")
		wantSQLState(t, ctx, db, "23503", "INSERT INTO user_profiles(user_id) VALUES ('20000000-0000-4000-8000-000000000099')")
		wantSQLState(t, ctx, db, "23514", "INSERT INTO user_profiles(user_id,website_url) VALUES ($1,'http://example.com')", alice)
		exec(t, ctx, db, "INSERT INTO user_profiles(user_id,city) VALUES ($1,'Sydney')", alice)
	})
	t.Run("self assessment constraints", func(t *testing.T) {
		exec(t, ctx, db, "INSERT INTO user_skills(user_id,skill_id,proficiency) VALUES ($1,$2,'learning')", alice, goSkill)
		wantSQLState(t, ctx, db, "23505", "INSERT INTO user_skills(user_id,skill_id,proficiency) VALUES ($1,$2,'working')", alice, goSkill)
		wantSQLState(t, ctx, db, "23514", "UPDATE user_skills SET proficiency='not_assessed' WHERE user_id=$1", alice)
		wantSQLState(t, ctx, db, "23503", "DELETE FROM skills WHERE id=$1", goSkill)
		if n := scalar(t, ctx, db, "SELECT count(*) FROM user_skills WHERE user_id=$1", bob); n != 0 {
			t.Fatal("another user's data appeared")
		}
	})
	exec(t, ctx, db, "INSERT INTO applications(id,owner_id,company,role_title) VALUES ($1,$2,'Example','Engineer')", job, alice)
	t.Run("status history and versions", func(t *testing.T) {
		if n := scalar(t, ctx, db, "SELECT count(*) FROM application_status_history WHERE application_id=$1 AND from_status IS NULL", job); n != 1 {
			t.Fatal(n)
		}
		exec(t, ctx, db, "UPDATE applications SET status='applied' WHERE id=$1 AND owner_id=$2 AND version=1", job, alice)
		if n := scalar(t, ctx, db, "SELECT version FROM applications WHERE id=$1", job); n != 2 {
			t.Fatal(n)
		}
		if n := scalar(t, ctx, db, "SELECT jd_version FROM applications WHERE id=$1", job); n != 1 {
			t.Fatal(n)
		}
		exec(t, ctx, db, "UPDATE applications SET job_description='Go SQL' WHERE id=$1", job)
		if n := scalar(t, ctx, db, "SELECT jd_version FROM applications WHERE id=$1", job); n != 2 {
			t.Fatal(n)
		}
		if n := scalar(t, ctx, db, "SELECT count(*) FROM application_status_history WHERE application_id=$1", job); n != 2 {
			t.Fatal(n)
		}
		wantSQLState(t, ctx, db, "23514", "UPDATE applications SET status='interview' WHERE id=$1", job)
		wantSQLState(t, ctx, db, "23514", "UPDATE applications SET owner_id=$2 WHERE id=$1", job, bob)
		wantSQLState(t, ctx, db, "23503", "INSERT INTO application_status_history(application_id,owner_id,to_status,application_version) VALUES ($1,$2,'offer',99)", job, bob)
		wantSQLState(t, ctx, db, "23514", "UPDATE applications SET job_description=repeat('界',21846) WHERE id=$1", job)
	})
	t.Run("owner and stale write predicates", func(t *testing.T) {
		for _, params := range [][]any{{job, bob, 3}, {job, alice, 1}} {
			r, err := db.ExecContext(ctx, "UPDATE applications SET notes='must not save' WHERE id=$1 AND owner_id=$2 AND version=$3", params...)
			if err != nil {
				t.Fatal(err)
			}
			n, _ := r.RowsAffected()
			if n != 0 {
				t.Fatal("unauthorized or stale predicate wrote data")
			}
		}
	})
	t.Run("two concurrent edits have exactly one winner", func(t *testing.T) {
		start := make(chan struct{})
		counts := make(chan int64, 2)
		failures := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, err := db.ExecContext(ctx, "UPDATE applications SET notes='edited' WHERE id=$1 AND owner_id=$2 AND version=3", job, alice)
				if err != nil {
					failures <- err
					return
				}
				n, err := r.RowsAffected()
				if err != nil {
					failures <- err
					return
				}
				counts <- n
			}()
		}
		close(start)
		wg.Wait()
		close(counts)
		close(failures)
		for err := range failures {
			t.Error(err)
		}
		var total int64
		for n := range counts {
			total += n
		}
		if total != 1 {
			t.Fatalf("winners=%d", total)
		}
	})
	t.Run("failed transaction rolls back status history and profile", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, "UPDATE applications SET status='offer' WHERE id=$1", job); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE users SET profile_version=profile_version+1 WHERE id=$1", alice); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE user_profiles SET city='Melbourne' WHERE user_id=$1", alice); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE user_skills SET proficiency='invalid' WHERE user_id=$1", alice); err == nil {
			t.Fatal("expected constraint failure")
		}
		if err = tx.Commit(); err == nil {
			t.Fatal("aborted transaction committed")
		}
		if n := scalar(t, ctx, db, "SELECT count(*) FROM application_status_history WHERE application_id=$1", job); n != 2 {
			t.Fatal(n)
		}
		if n := scalar(t, ctx, db, "SELECT profile_version FROM users WHERE id=$1", alice); n != 1 {
			t.Fatal(n)
		}
		if n := scalar(t, ctx, db, "SELECT count(*) FROM user_profiles WHERE user_id=$1 AND city='Sydney'", alice); n != 1 {
			t.Fatal(n)
		}
	})
	// Remove only our fixtures, then prove down-all / up-all including reference seeds.
	exec(t, ctx, db, "DELETE FROM applications WHERE id=$1", job)
	exec(t, ctx, db, "DELETE FROM users WHERE id IN ($1,$2)", alice, bob)
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, ctx, db, "SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('users','applications','skills')"); n != 0 {
		t.Fatal("down left tables")
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, ctx, db, "SELECT count(*) FROM skills"); n != 8 {
		t.Fatal("seed replay failed")
	}
}
