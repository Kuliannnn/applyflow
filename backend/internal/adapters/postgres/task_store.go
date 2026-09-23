package postgres

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const taskColumns = `id,kind,status,stage,state_version,attempts,cancel_requested,result,safe_error_code,created_at,finished_at`

func scanTask(row scanner) (task.Snapshot, error) {
	var t task.Snapshot
	var raw []byte
	err := row.Scan(&t.ID, &t.Kind, &t.Status, &t.Stage, &t.StateVersion, &t.Attempts, &t.CancelRequested, &raw, &t.ErrorCode, &t.CreatedAt, &t.FinishedAt)
	t.Result = raw
	return t, notFound(err)
}
func (s FlowStore) Task(ctx context.Context, owner, id string) (task.Snapshot, error) {
	return scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM job_tasks WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s FlowStore) Cancel(ctx context.Context, owner, id string) (task.Snapshot, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return task.Snapshot{}, err
	}
	defer tx.Rollback()
	t, err := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM job_tasks WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return t, err
	}
	if t.Status == "completed" || t.Status == "failed" || t.Status == "cancelled" {
		return t, nil
	}
	// Finish cancellation immediately: all result commits are fenced by status.
	// This also avoids renewing an already-expired running lease.
	t, err = scanTask(tx.QueryRowContext(ctx, `UPDATE job_tasks SET status='cancelled',stage='cancelled',cancel_requested=true,lease_until=NULL,next_attempt_at=NULL,finished_at=clock_timestamp() WHERE owner_id=$1 AND id=$2 RETURNING `+taskColumns, owner, id))
	if err != nil {
		return t, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE task_outbox SET published_at=clock_timestamp(),claim_token=NULL,claim_until=NULL WHERE task_id=$1 AND published_at IS NULL`, id); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

// Claim consumes a durable outbox delivery in PostgreSQL for the mock runtime.
// It is replaceable by Redis/Asynq delivery without changing task/result semantics.
func (s FlowStore) Claim(ctx context.Context) (*task.Claim, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM job_tasks t WHERE t.status='queued' AND NOT t.cancel_requested AND t.kind IN ('extract_jd','tailor_resume','write_cover_letter','export_document') AND EXISTS(SELECT 1 FROM task_outbox o WHERE o.task_id=t.id AND o.published_at IS NULL AND o.next_dispatch_at<=clock_timestamp()) ORDER BY t.created_at,t.id LIMIT 1 FOR UPDATE OF t SKIP LOCKED`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c task.Claim
	err = tx.QueryRowContext(ctx, `UPDATE job_tasks SET status='running',stage=CASE WHEN kind='extract_jd' THEN 'extracting' WHEN kind='export_document' THEN 'exporting' ELSE 'generating' END,attempts=attempts+1,fencing_token=fencing_token+1,lease_until=clock_timestamp()+interval '30 seconds',safe_error_code=NULL WHERE id=$1 RETURNING id,owner_id,kind,workspace_id,document_id,run_id,source_id,fencing_token,expected_document_version`, id).Scan(&c.ID, &c.OwnerID, &c.Kind, &c.WorkspaceID, &c.DocumentID, &c.RunID, &c.SourceID, &c.Fence, &c.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE task_outbox SET published_at=clock_timestamp(),claim_token=NULL,claim_until=NULL WHERE task_id=$1 AND published_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	return &c, tx.Commit()
}
func (s FlowStore) Recover(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Bounded SKIP LOCKED recovery. No external IO or workspace locks in this transaction.
	rows, err := tx.QueryContext(ctx, `SELECT id,owner_id,status,attempts,max_attempts,cancel_requested FROM job_tasks WHERE kind IN ('extract_jd','tailor_resume','write_cover_letter','export_document') AND ((status='running' AND lease_until<=clock_timestamp()) OR (status='retry_wait' AND next_attempt_at<=clock_timestamp())) ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type recovery struct {
		id, owner, status string
		attempts, max     int
		cancel            bool
	}
	var batch []recovery
	for rows.Next() {
		var r recovery
		if err = rows.Scan(&r.id, &r.owner, &r.status, &r.attempts, &r.max, &r.cancel); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range batch {
		switch {
		case r.cancel || r.attempts >= r.max:
			state, code := "failed", "attempts_exhausted"
			if r.cancel {
				state, code = "cancelled", "cancelled"
			}
			_, err = tx.ExecContext(ctx, `UPDATE job_tasks SET status=$2,stage=$2,lease_until=NULL,next_attempt_at=NULL,finished_at=clock_timestamp(),safe_error_code=$3 WHERE id=$1`, r.id, state, code)
		case r.status == "running":
			_, err = tx.ExecContext(ctx, `UPDATE job_tasks SET status='retry_wait',stage='retry_wait',lease_until=NULL,next_attempt_at=clock_timestamp()+interval '1 second',safe_error_code='lease_expired' WHERE id=$1`, r.id)
		default:
			_, err = tx.ExecContext(ctx, `UPDATE job_tasks SET status='queued',stage='queued',next_attempt_at=NULL WHERE id=$1`, r.id)
			if err == nil {
				_, err = tx.ExecContext(ctx, `INSERT INTO task_outbox(owner_id,task_id,delivery_generation) VALUES($1,$2,$3) ON CONFLICT(task_id,delivery_generation) DO NOTHING`, r.owner, r.id, r.attempts+1)
			}
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s FlowStore) Fail(ctx context.Context, c task.Claim, code string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE job_tasks SET status='failed',stage='failed',lease_until=NULL,finished_at=clock_timestamp(),safe_error_code=$4 WHERE owner_id=$1 AND id=$2 AND status='running' AND fencing_token=$3 AND lease_until>clock_timestamp() AND NOT cancel_requested`, c.OwnerID, c.ID, c.Fence, code)
	return err
}
func completeTask(ctx context.Context, tx *sql.Tx, c task.Claim, result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `UPDATE job_tasks SET status='completed',stage='completed',lease_until=NULL,finished_at=clock_timestamp(),result=$4 WHERE owner_id=$1 AND id=$2 AND status='running' AND fencing_token=$3 AND lease_until>clock_timestamp() AND NOT cancel_requested`, c.OwnerID, c.ID, c.Fence, raw)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return task.ErrLeaseLost
	}
	return nil
}
func (s FlowStore) LoadExecution(ctx context.Context, c task.Claim) (generation.Input, error) {
	var in generation.Input
	if c.Kind == "extract_jd" {
		if c.SourceID == nil {
			return in, studio.ErrInvalid
		}
		source, err := s.Source(ctx, c.OwnerID, c.WorkspaceID, *c.SourceID)
		if err != nil {
			return in, err
		}
		if source.Source.Kind != "text" {
			return in, studio.ErrInvalid
		}
		in.Text = source.Source.Text
		return in, nil
	}
	if c.RunID == nil || c.DocumentID == nil || c.ExpectedVersion == nil {
		return in, studio.ErrInvalid
	}
	var err error
	in.Facts, err = runFacts(ctx, s.DB, c.OwnerID, *c.RunID)
	if err != nil {
		return in, err
	}
	err = s.DB.QueryRowContext(ctx, `SELECT j.role_title,j.company FROM generation_runs g JOIN job_revisions j ON j.id=g.job_revision_id AND j.owner_id=g.owner_id WHERE g.owner_id=$1 AND g.id=$2 AND g.execution_mode='mock' AND g.prompt_version='mock-v1'`, c.OwnerID, *c.RunID).Scan(&in.Role, &in.Company)
	return in, err
}
func (s FlowStore) CompleteExtraction(ctx context.Context, c task.Claim, text string) error {
	result := map[string]any{"kind": "job_extraction", "company": nil, "role_title": nil, "job_description": text, "uncertainties": []any{}}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = completeTask(ctx, tx, c, result); err != nil {
		return err
	}
	return tx.Commit()
}
func (s FlowStore) CompleteDocument(ctx context.Context, c task.Claim, content document.Content) error {
	raw, err := json.Marshal(content)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Always acquire workspace -> document -> task. Human saves use the same order.
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, c.OwnerID, c.WorkspaceID))
	if err != nil {
		return err
	}
	if w.ArchivedAt != nil {
		return studio.ErrArchived
	}
	d, err := scanDocument(tx.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND id=$2 FOR UPDATE`, c.OwnerID, *c.DocumentID))
	if err != nil {
		return err
	}
	revisionID := security.UUID()
	if err = completeTask(ctx, tx, c, map[string]any{"kind": "document", "document_id": d.ID, "revision_id": revisionID}); err != nil {
		return err
	}
	var input []byte
	err = tx.QueryRowContext(ctx, `SELECT input_snapshot FROM job_tasks WHERE id=$1 AND owner_id=$2`, c.ID, c.OwnerID).Scan(&input)
	if err != nil {
		return err
	}
	var snapshot struct {
		Base *string `json:"base_revision_id"`
	}
	if err = json.Unmarshal(input, &snapshot); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO document_revisions(id,owner_id,document_id,kind,parent_revision_id,run_id,task_id,origin,content,change_summary) VALUES($1,$2,$3,$4,$5,$6,$7,'ai',$8,'Mock draft — no external AI used')`, revisionID, c.OwnerID, d.ID, d.Kind, snapshot.Base, *c.RunID, c.ID, raw)
	if err != nil {
		return err
	}
	if d.CurrentRevisionID == nil && d.Version == *c.ExpectedVersion && w.CurrentRunID != nil && *w.CurrentRunID == *c.RunID {
		if _, err = tx.ExecContext(ctx, `UPDATE documents SET current_revision_id=$3 WHERE owner_id=$1 AND id=$2 AND version=$4`, c.OwnerID, d.ID, revisionID, *c.ExpectedVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}
