package postgres

import (
	"applyflow/backend/internal/studio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type FlowStore struct{ DB *sql.DB }

func hashJSON(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// Caller holds the owner row; replay precedes all mutable version checks.
func replay[T any](ctx context.Context, tx *sql.Tx, owner, operation, key string, in any) (T, bool, error) {
	var out T
	var previous string
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_requests WHERE owner_id=$1 AND operation=$2 AND key_hash=$3`, owner, operation, hashJSON(key)).Scan(&previous, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if previous != hashJSON(in) {
		return out, false, studio.ErrKeyConflict
	}
	err = json.Unmarshal(raw, &out)
	return out, true, err
}
func saveReplay(ctx context.Context, tx *sql.Tx, owner, operation, key string, in, out any) error {
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES($1,$2,$3,$4,202,$5,now()+interval '24 hours')`, owner, operation, hashJSON(key), hashJSON(in), raw)
	return err
}
func lockWorkspace(ctx context.Context, tx *sql.Tx, owner, id string, version int64) (studio.Workspace, error) {
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return w, err
	}
	if w.ArchivedAt != nil {
		return w, studio.ErrArchived
	}
	if w.Version != version {
		return w, studio.ErrConflict
	}
	return w, nil
}
func addOutbox(ctx context.Context, tx *sql.Tx, owner, task string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO task_outbox(owner_id,task_id) VALUES($1,$2)`, owner, task)
	return err
}
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return studio.ErrNotFound
	}
	return err
}
