package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"applyflow/backend/internal/studio"
)

type WorkspaceStore struct{ DB *sql.DB }

const workspaceColumns = `id,title,version,current_source_id,job_revision_id,resume_revision_id,current_run_id,application_id,archived_at,created_at,updated_at`

type scanner interface{ Scan(...any) error }

func scanWorkspace(row scanner) (studio.Workspace, error) {
	var w studio.Workspace
	err := row.Scan(&w.ID, &w.Title, &w.Version, &w.CurrentSourceID, &w.JobRevisionID, &w.ResumeRevisionID, &w.CurrentRunID, &w.ApplicationID, &w.ArchivedAt, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = studio.ErrNotFound
	}
	return w, err
}
func (s WorkspaceStore) Create(ctx context.Context, owner, title string, key, hash [32]byte) (studio.Workspace, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return studio.Workspace{}, err
	}
	defer tx.Rollback()
	// Per-user serialization makes concurrent replay and creation one atomic decision.
	var userID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&userID); err != nil {
		return studio.Workspace{}, err
	}
	kh, rh := hex.EncodeToString(key[:]), hex.EncodeToString(hash[:])
	var priorHash string
	var body []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_requests WHERE owner_id=$1 AND operation='create_workspace' AND key_hash=$2`, owner, kh).Scan(&priorHash, &body)
	if err == nil {
		if priorHash != rh {
			return studio.Workspace{}, studio.ErrKeyConflict
		}
		var w studio.Workspace
		if err = json.Unmarshal(body, &w); err != nil {
			return w, err
		}
		return w, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return studio.Workspace{}, err
	}
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `INSERT INTO workspaces(owner_id,title) VALUES ($1,$2) RETURNING `+workspaceColumns, owner, title))
	if err != nil {
		return w, err
	}
	body, err = json.Marshal(w)
	if err != nil {
		return w, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES ($1,'create_workspace',$2,$3,201,$4,now()+interval '24 hours')`, owner, kh, rh, body)
	if err != nil {
		return w, err
	}
	return w, tx.Commit()
}
func (s WorkspaceStore) Get(ctx context.Context, owner, id string) (studio.Workspace, error) {
	return scanWorkspace(s.DB.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s WorkspaceStore) List(ctx context.Context, owner string, limit int, after *studio.Boundary) ([]studio.Workspace, error) {
	var stamp, id any
	if after != nil {
		stamp, id = after.UpdatedAt, after.ID
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND archived_at IS NULL AND ($2::timestamptz IS NULL OR (updated_at,id)<($2::timestamptz,$3::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $4`, owner, stamp, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]studio.Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, w)
	}
	return items, rows.Err()
}
func (s WorkspaceStore) Update(ctx context.Context, owner, id string, p studio.Patch) (studio.Workspace, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return studio.Workspace{}, err
	}
	defer tx.Rollback()
	// Owner row first: consistent with later source/default-resume mutations.
	var userID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&userID); err != nil {
		return studio.Workspace{}, err
	}
	old, err := scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return old, err
	}
	if old.ArchivedAt != nil {
		return old, studio.ErrArchived
	}
	if old.Version != p.ExpectedVersion {
		return old, studio.ErrConflict
	}
	if p.SetResume && p.ResumeRevisionID != nil {
		var found bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM resume_revisions rr JOIN resumes r ON r.id=rr.resume_id AND r.owner_id=rr.owner_id JOIN files f ON f.id=r.source_file_id AND f.owner_id=r.owner_id WHERE rr.id=$1 AND rr.owner_id=$2 AND f.state='ready')`, *p.ResumeRevisionID, owner).Scan(&found)
		if err != nil {
			return old, err
		}
		if !found {
			return studio.Workspace{}, studio.ErrNotFound
		}
	}
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `UPDATE workspaces SET title=COALESCE($4,title),resume_revision_id=CASE WHEN $5 THEN $6::uuid ELSE resume_revision_id END WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING `+workspaceColumns, owner, id, p.ExpectedVersion, p.Title, p.SetResume, p.ResumeRevisionID))
	if err != nil {
		return w, err
	}
	return w, tx.Commit()
}
