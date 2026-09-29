package postgres

import (
	"context"
	"database/sql"

	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/task"
)

// Caller holds the user lock. Reservations for cancelled/failed unsent tasks
// are excluded. Dispatched attempts count even when their outcome is unknown.
func checkAIBudget(ctx context.Context, tx *sql.Tx, owner string, slots int, exclude string) error {
	var limit, used int
	err := tx.QueryRowContext(ctx, `SELECT daily_request_limit FROM user_ai_settings WHERE user_id=$1`, owner).Scan(&limit)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_generation_usage u JOIN job_tasks t ON t.id=u.task_id WHERE u.owner_id=$1 AND u.task_id::text<>$2 AND COALESCE(u.started_at,u.created_at)>clock_timestamp()-interval '24 hours' AND (u.state<>'reserved' OR t.status IN ('queued','running','retry_wait'))`, owner, exclude).Scan(&used)
	if err != nil {
		return err
	}
	if used+slots > limit {
		return generation.Failure("ai_daily_limit")
	}
	return nil
}
func currentAIRevision(ctx context.Context, tx *sql.Tx, owner string, revision *int64) (string, error) {
	if revision == nil {
		return "", generation.Failure("ai_config_required")
	}
	var id string
	err := tx.QueryRowContext(ctx, `SELECT r.id FROM user_ai_settings s JOIN user_ai_config_revisions r ON r.id=s.current_revision_id AND r.owner_id=s.user_id JOIN ai_credentials c ON c.id=r.credential_id AND c.owner_id=r.owner_id WHERE s.user_id=$1 AND r.revision=$2 AND s.enabled AND r.test_status='succeeded' AND r.revoked_at IS NULL AND c.revoked_at IS NULL`, owner, *revision).Scan(&id)
	if err == sql.ErrNoRows {
		return "", generation.Failure("ai_config_required")
	}
	return id, err
}
func (s FlowStore) BeginCall(ctx context.Context, c task.Claim) (generation.Credential, error) {
	var out generation.Credential
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, c.OwnerID); err != nil {
		return out, err
	}
	var state string
	err = tx.QueryRowContext(ctx, `SELECT u.state FROM ai_generation_usage u JOIN job_tasks t ON t.id=u.task_id AND t.owner_id=u.owner_id WHERE u.task_id=$1 AND u.owner_id=$2 AND t.status='running' AND t.fencing_token=$3 AND t.lease_until>clock_timestamp() AND NOT t.cancel_requested FOR UPDATE OF t,u`, c.ID, c.OwnerID, c.Fence).Scan(&state)
	if err == sql.ErrNoRows {
		return out, task.ErrLeaseLost
	}
	if err != nil {
		return out, err
	}
	if state != "reserved" {
		return out, generation.Failure("provider_outcome_unknown")
	}
	// Pinned credentials may survive model replacement; disabling/deleting stops
	// unsent calls. Explicit retries use the same run binding, never a new key.
	err = tx.QueryRowContext(ctx, `SELECT c.id,r.model_id,c.ciphertext,c.nonce,c.key_version FROM ai_generation_usage u JOIN user_ai_config_revisions r ON r.id=u.revision_id AND r.owner_id=u.owner_id JOIN ai_credentials c ON c.id=r.credential_id AND c.owner_id=r.owner_id JOIN user_ai_settings s ON s.user_id=u.owner_id WHERE u.task_id=$1 AND u.owner_id=$2 AND s.enabled AND r.test_status='succeeded' AND r.revoked_at IS NULL AND c.revoked_at IS NULL`, c.ID, c.OwnerID).Scan(&out.ID, &out.Model, &out.Sealed.Ciphertext, &out.Sealed.Nonce, &out.Sealed.KeyVersion)
	if err == sql.ErrNoRows {
		return out, generation.Failure("ai_config_required")
	}
	if err != nil {
		return out, err
	}
	if err = checkAIBudget(ctx, tx, c.OwnerID, 1, c.ID); err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_generation_usage SET state='started',started_at=clock_timestamp() WHERE task_id=$1`, c.ID)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) FinishCall(ctx context.Context, c task.Claim, usage generation.Usage, state string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE ai_generation_usage SET state=$3,input_tokens=$4,output_tokens=$5,finished_at=clock_timestamp() WHERE task_id=$1 AND owner_id=$2 AND state='started'`, c.ID, c.OwnerID, state, usage.InputTokens, usage.OutputTokens)
	return err
}

func (s AISettingsStore) GenerationUsage(ctx context.Context, owner string) (aisettings.GenerationUsage, error) {
	var out aisettings.GenerationUsage
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT daily_request_limit FROM user_ai_settings WHERE user_id=$1),20),count(*) FILTER(WHERE u.state='reserved'),count(*) FILTER(WHERE u.state<>'reserved'),count(*) FILTER(WHERE u.state<>'reserved' AND u.input_tokens IS NULL),COALESCE(sum(u.input_tokens),0),COALESCE(sum(u.output_tokens),0) FROM ai_generation_usage u JOIN job_tasks t ON t.id=u.task_id WHERE u.owner_id=$1 AND COALESCE(u.started_at,u.created_at)>clock_timestamp()-interval '24 hours' AND (u.state<>'reserved' OR t.status IN ('queued','running','retry_wait'))`, owner).Scan(&out.DailyLimit, &out.Reserved, &out.Attempts, &out.Unknown, &out.InputTokens, &out.OutputTokens)
	return out, err
}
