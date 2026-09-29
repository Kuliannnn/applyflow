package postgres

import (
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/studio"
	"context"
	"database/sql"
	"encoding/json"
)

func generationTask(ctx context.Context, tx *sql.Tx, owner, workspace, run string, doc document.Document) (string, error) {
	kind := "tailor_resume"
	if doc.Kind == "cover_letter" {
		kind = "write_cover_letter"
	}
	raw, _ := json.Marshal(map[string]any{"base_revision_id": doc.CurrentRevisionID})
	var id string
	err := tx.QueryRowContext(ctx, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,run_id,kind,expected_document_version,input_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, owner, workspace, doc.ID, run, kind, doc.Version, raw).Scan(&id)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_generation_usage(task_id,owner_id,revision_id) SELECT $1,owner_id,ai_revision_id FROM generation_runs WHERE id=$2 AND owner_id=$3 AND execution_mode='personal'`, id, run, owner); err != nil {
		return "", err
	}
	return id, addOutbox(ctx, tx, owner, id)
}
func activeDocuments(ctx context.Context, tx *sql.Tx, owner, id string) (bool, error) {
	var active bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM job_tasks WHERE owner_id=$1 AND workspace_id=$2 AND document_id IS NOT NULL AND kind IN ('tailor_resume','write_cover_letter','revise_document') AND status IN ('queued','running','retry_wait'))`, owner, id).Scan(&active)
	return active, err
}
func (s FlowStore) Generate(ctx context.Context, owner, id, key string, in studio.Generate) (studio.GenerationAccepted, error) {
	var out studio.GenerationAccepted
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	op := "generate:" + id
	if prior, ok, err := replay[studio.GenerationAccepted](ctx, tx, owner, op, key, in); err != nil || ok {
		return prior, err
	}
	var aiID *string
	prompt := "mock-v1"
	if in.ExecutionMode == "personal" {
		binding, e := currentAIRevision(ctx, tx, owner, in.AIRevision)
		if e != nil {
			return out, e
		}
		if e = checkAIBudget(ctx, tx, owner, 2, ""); e != nil {
			return out, e
		}
		aiID = &binding
		prompt = "personal-v1"
	}
	w, err := lockWorkspace(ctx, tx, owner, id, in.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if w.JobRevisionID == nil || w.ResumeRevisionID == nil || *w.JobRevisionID != in.JobRevisionID || *w.ResumeRevisionID != in.ResumeRevisionID {
		return out, studio.ErrConflict
	}
	var profile int64
	err = tx.QueryRowContext(ctx, `SELECT profile_version FROM users WHERE id=$1`, owner).Scan(&profile)
	if err != nil {
		return out, err
	}
	if profile != in.ProfileVersion {
		return out, studio.ErrConflict
	}
	active, err := activeDocuments(ctx, tx, owner, id)
	if err != nil {
		return out, err
	}
	if active {
		return out, studio.ErrConflict
	}
	// No personal profile fields are used by the mock provider; snapshot intentionally empty.
	err = tx.QueryRowContext(ctx, `INSERT INTO generation_runs(owner_id,workspace_id,job_revision_id,resume_revision_id,profile_version,profile_snapshot,execution_mode,locale,prompt_version,ai_revision_id) VALUES($1,$2,$3,$4,$5,'{}',$6,'en',$7,$8) RETURNING id`, owner, id, in.JobRevisionID, in.ResumeRevisionID, in.ProfileVersion, in.ExecutionMode, prompt, aiID).Scan(&out.RunID)
	if err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO documents(owner_id,workspace_id,kind) VALUES($1,$2,'resume'),($1,$2,'cover_letter') ON CONFLICT(workspace_id,kind) DO NOTHING`, owner, id)
	if err != nil {
		return out, err
	}
	docs, err := queryDocuments(ctx, tx, owner, id, true)
	if err != nil {
		return out, err
	}
	for _, doc := range docs {
		taskID, err := generationTask(ctx, tx, owner, id, out.RunID, doc)
		if err != nil {
			return out, err
		}
		if doc.Kind == "resume" {
			out.ResumeTaskID = taskID
		} else {
			out.CoverLetterTaskID = taskID
		}
	}
	out.WorkspaceID = id
	err = tx.QueryRowContext(ctx, `UPDATE workspaces SET current_run_id=$3 WHERE owner_id=$1 AND id=$2 RETURNING version`, owner, id, out.RunID).Scan(&out.WorkspaceVersion)
	if err != nil {
		return out, err
	}
	if err = saveReplay(ctx, tx, owner, op, key, in, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) Generation(ctx context.Context, owner, id, run string) (studio.Generation, error) {
	var out studio.Generation
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT id,workspace_id,job_revision_id,resume_revision_id,profile_version,execution_mode,locale,created_at FROM generation_runs WHERE owner_id=$1 AND workspace_id=$2 AND id=$3`, owner, id, run).Scan(&out.ID, &out.WorkspaceID, &out.JobRevisionID, &out.ResumeRevisionID, &out.ProfileVersion, &out.ExecutionMode, &out.Locale, &out.CreatedAt)
	if err != nil {
		return out, notFound(err)
	}
	if out.ExecutionMode == "personal" {
		err = tx.QueryRowContext(ctx, `SELECT r.provider_id,r.model_id,r.revision FROM generation_runs g JOIN user_ai_config_revisions r ON r.id=g.ai_revision_id AND r.owner_id=g.owner_id WHERE g.id=$1 AND g.owner_id=$2`, run, owner).Scan(&out.ProviderID, &out.ModelID, &out.AIRevision)
		if err != nil {
			return out, err
		}
	}
	out.ResumeTask, err = scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM job_tasks WHERE owner_id=$1 AND run_id=$2 AND kind='tailor_resume' ORDER BY created_at DESC,id DESC LIMIT 1`, owner, run))
	if err != nil {
		return out, err
	}
	out.CoverLetterTask, err = scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM job_tasks WHERE owner_id=$1 AND run_id=$2 AND kind='write_cover_letter' ORDER BY created_at DESC,id DESC LIMIT 1`, owner, run))
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) RetryGeneration(ctx context.Context, owner, id, run, key string, in studio.Retry) (studio.TaskAccepted, error) {
	var out studio.TaskAccepted
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	op := "retry:" + id + ":" + run
	if prior, ok, err := replay[studio.TaskAccepted](ctx, tx, owner, op, key, in); err != nil || ok {
		return prior, err
	}
	var mode string
	var bound *int64
	err = tx.QueryRowContext(ctx, `SELECT g.execution_mode,r.revision FROM generation_runs g LEFT JOIN user_ai_config_revisions r ON r.id=g.ai_revision_id AND r.owner_id=g.owner_id WHERE g.id=$1 AND g.owner_id=$2 AND g.workspace_id=$3`, run, owner, id).Scan(&mode, &bound)
	if err != nil {
		return out, notFound(err)
	}
	if mode == "personal" {
		if _, e := currentAIRevision(ctx, tx, owner, bound); e != nil {
			return out, generation.Failure("ai_config_required")
		}
		if e := checkAIBudget(ctx, tx, owner, 1, ""); e != nil {
			return out, e
		}
	}
	w, err := lockWorkspace(ctx, tx, owner, id, in.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if w.CurrentRunID == nil || *w.CurrentRunID != run {
		return out, studio.ErrConflict
	}
	doc, err := scanDocument(tx.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND workspace_id=$2 AND kind=$3 FOR UPDATE`, owner, id, in.Kind))
	if err != nil {
		return out, err
	}
	var state string
	err = tx.QueryRowContext(ctx, `SELECT status FROM job_tasks WHERE owner_id=$1 AND run_id=$2 AND document_id=$3 AND kind IN ('tailor_resume','write_cover_letter') ORDER BY created_at DESC,id DESC LIMIT 1`, owner, run, doc.ID).Scan(&state)
	if err != nil {
		return out, notFound(err)
	}
	if state != "failed" && state != "cancelled" {
		return out, studio.ErrConflict
	}
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM job_tasks WHERE document_id=$1 AND status IN ('queued','running','retry_wait'))`, doc.ID).Scan(&active)
	if err != nil {
		return out, err
	}
	if active {
		return out, studio.ErrConflict
	}
	out.TaskID, err = generationTask(ctx, tx, owner, id, run, doc)
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET current_run_id=current_run_id WHERE owner_id=$1 AND id=$2`, owner, id); err != nil {
		return out, err
	}
	if err = saveReplay(ctx, tx, owner, op, key, in, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
