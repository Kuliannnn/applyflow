package postgres

import (
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
	"context"
	"encoding/json"
	"errors"
)

const exportColumns = `id,document_id,document_revision_id,format,template_version,task_id,file_id,created_at`

func scanExport(row scanner) (export.Export, error) {
	var x export.Export
	err := row.Scan(&x.ID, &x.DocumentID, &x.RevisionID, &x.Format, &x.TemplateVersion, &x.TaskID, &x.FileID, &x.CreatedAt)
	return x, notFound(err)
}
func (s FlowStore) CreateExport(ctx context.Context, owner, id, key string, in export.Create) (export.Accepted, error) {
	var out export.Accepted
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	op := "export:" + id
	if prior, ok, err := replay[export.Accepted](ctx, tx, owner, op, key, in); err != nil || ok {
		return prior, err
	}
	// Resolve owner before taking workspace -> document locks. Export targets may be historical.
	var workspace string
	err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM documents WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&workspace)
	if err != nil {
		return out, notFound(err)
	}
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, workspace))
	if err != nil {
		return out, err
	}
	if w.ArchivedAt != nil {
		return out, studio.ErrArchived
	}
	d, err := scanDocument(tx.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return out, err
	}
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM document_revisions WHERE owner_id=$1 AND document_id=$2 AND id=$3 AND kind=$4)`, owner, id, in.RevisionID, d.Kind).Scan(&exists)
	if err != nil {
		return out, err
	}
	if !exists {
		return out, studio.ErrNotFound
	}
	x, err := scanExport(tx.QueryRowContext(ctx, `SELECT `+exportColumns+` FROM document_exports WHERE owner_id=$1 AND document_id=$2 AND document_revision_id=$3 AND format=$4 AND template_version=$5`, owner, id, in.RevisionID, in.Format, in.TemplateVersion))
	reuse := err == nil
	if err != nil && !errors.Is(err, studio.ErrNotFound) {
		return out, err
	}
	out.Status = 202
	needsTask := true
	if reuse {
		state, err := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM job_tasks WHERE id=$1 AND owner_id=$2`, x.TaskID, owner))
		if err != nil {
			return out, err
		}
		if state.Status == "completed" && x.FileID != nil {
			out.Status = 200
			needsTask = false
		} else if state.Status == "queued" || state.Status == "running" || state.Status == "retry_wait" {
			needsTask = false
		}
	}
	if needsTask {
		raw, _ := json.Marshal(in)
		var taskID string
		err = tx.QueryRowContext(ctx, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,kind,input_snapshot) VALUES($1,$2,$3,'export_document',$4) RETURNING id`, owner, workspace, id, raw).Scan(&taskID)
		if err != nil {
			return out, err
		}
		if err = addOutbox(ctx, tx, owner, taskID); err != nil {
			return out, err
		}
		if reuse {
			x, err = scanExport(tx.QueryRowContext(ctx, `UPDATE document_exports SET task_id=$3,file_id=NULL WHERE owner_id=$1 AND id=$2 RETURNING `+exportColumns, owner, x.ID, taskID))
		} else {
			x, err = scanExport(tx.QueryRowContext(ctx, `INSERT INTO document_exports(owner_id,document_id,document_revision_id,format,template_version,task_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+exportColumns, owner, id, in.RevisionID, in.Format, in.TemplateVersion, taskID))
		}
		if err != nil {
			return out, err
		}
	}
	out.Export = x
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES($1,$2,$3,$4,$5,$6,now()+interval '24 hours')`, owner, op, hashJSON(key), hashJSON(in), out.Status, raw)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) Export(ctx context.Context, owner, id, x string) (export.Export, error) {
	return scanExport(s.DB.QueryRowContext(ctx, `SELECT `+exportColumns+` FROM document_exports WHERE owner_id=$1 AND document_id=$2 AND id=$3`, owner, id, x))
}
func (s FlowStore) LoadExport(ctx context.Context, c task.Claim) (export.Export, document.Revision, error) {
	var r document.Revision
	x, err := scanExport(s.DB.QueryRowContext(ctx, `SELECT `+exportColumns+` FROM document_exports WHERE owner_id=$1 AND task_id=$2`, c.OwnerID, c.ID))
	if err != nil {
		return x, r, err
	}
	r, err = s.DocumentRevision(ctx, c.OwnerID, x.DocumentID, x.RevisionID)
	return x, r, err
}
func (s FlowStore) CompleteExport(ctx context.Context, c task.Claim, x export.Export, f files.File) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	w, err := scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, c.OwnerID, c.WorkspaceID))
	if err != nil {
		return err
	}
	if w.ArchivedAt != nil {
		return studio.ErrArchived
	}
	if _, err = scanDocument(tx.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND id=$2 FOR UPDATE`, c.OwnerID, x.DocumentID)); err != nil {
		return err
	}
	if err = completeTask(ctx, tx, c, map[string]any{"kind": "export", "export_id": x.ID, "file_id": f.ID}); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO files(id,owner_id,purpose,state,original_name,storage_key,media_type,byte_size,sha256) VALUES($1,$2,'export','ready',$3,$4,$5,$6,$7)`, f.ID, c.OwnerID, f.OriginalName, f.StorageKey, f.MediaType, f.ByteSize, f.SHA256)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE document_exports SET file_id=$4 WHERE owner_id=$1 AND id=$2 AND task_id=$3 AND file_id IS NULL`, c.OwnerID, x.ID, c.ID, f.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return task.ErrLeaseLost
	}
	return tx.Commit()
}

// Keep interface changes visible at compile time.
var _ export.Store = FlowStore{}
