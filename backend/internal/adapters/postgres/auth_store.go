package postgres

import (
	"context"
	"database/sql"
	"errors"

	"applyflow/backend/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

type AuthStore struct{ DB *sql.DB }

func (s AuthStore) Create(ctx context.Context, email, hash string) (auth.Identity, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return auth.Identity{}, err
	}
	defer tx.Rollback()
	var u auth.Identity
	err = tx.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES ($1,$2) RETURNING id,email,profile_version,created_at`, email, hash).Scan(&u.ID, &u.Email, &u.ProfileVersion, &u.CreatedAt)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return u, auth.ErrEmailTaken
	}
	if err != nil {
		return u, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_profiles(user_id) VALUES ($1)`, u.ID); err != nil {
		return u, err
	}
	return u, tx.Commit()
}
func (s AuthStore) ByEmail(ctx context.Context, email string) (auth.Account, error) {
	var a auth.Account
	err := s.DB.QueryRowContext(ctx, `SELECT id,email,profile_version,created_at,password_hash FROM users WHERE email=$1`, email).Scan(&a.ID, &a.Email, &a.ProfileVersion, &a.CreatedAt, &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		err = auth.ErrNotFound
	}
	return a, err
}
func (s AuthStore) ByID(ctx context.Context, id string) (auth.Identity, error) {
	var a auth.Identity
	err := s.DB.QueryRowContext(ctx, `SELECT id,email,profile_version,created_at FROM users WHERE id=$1`, id).Scan(&a.ID, &a.Email, &a.ProfileVersion, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = auth.ErrNotFound
	}
	return a, err
}
