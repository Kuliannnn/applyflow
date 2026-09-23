package postgres

import (
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/studio"
	"context"
)

const sourceColumns = `id,workspace_id,source_version,kind,raw_text,created_at`

func scanSource(row scanner) (intake.Source, error) {
	var v intake.Source
	err := row.Scan(&v.ID, &v.WorkspaceID, &v.Version, &v.Source.Kind, &v.Source.Text, &v.CreatedAt)
	return v, notFound(err)
}
func (s FlowStore) CreateSource(ctx context.Context, owner, id, key string, in intake.Create) (intake.Accepted, error) {
	var out intake.Accepted
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	op := "create_source:" + id
	if prior, ok, err := replay[intake.Accepted](ctx, tx, owner, op, key, in); err != nil || ok {
		if err == nil {
			prior.Source, err = scanSource(tx.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM job_sources WHERE owner_id=$1 AND workspace_id=$2 AND id=$3`, owner, id, prior.Source.ID))
		}
		return prior, err
	}
	if _, err = lockWorkspace(ctx, tx, owner, id, in.ExpectedVersion); err != nil {
		return out, err
	}
	out.Source, err = scanSource(tx.QueryRowContext(ctx, `INSERT INTO job_sources(owner_id,workspace_id,source_version,kind,raw_text) VALUES($1,$2,(SELECT COALESCE(MAX(source_version),0)+1 FROM job_sources WHERE workspace_id=$2),'text',$3) RETURNING `+sourceColumns, owner, id, in.Source.Text))
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO job_tasks(owner_id,workspace_id,source_id,kind) VALUES($1,$2,$3,'extract_jd') RETURNING id`, owner, id, out.Source.ID).Scan(&out.TaskID)
	if err != nil {
		return out, err
	}
	if err = addOutbox(ctx, tx, owner, out.TaskID); err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE workspaces SET current_source_id=$3,job_revision_id=NULL WHERE owner_id=$1 AND id=$2 RETURNING version`, owner, id, out.Source.ID).Scan(&out.WorkspaceVersion)
	if err != nil {
		return out, err
	}
	compact := out
	compact.Source.Source.Text = ""
	if err = saveReplay(ctx, tx, owner, op, key, in, compact); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) Source(ctx context.Context, owner, id, source string) (intake.Source, error) {
	return scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM job_sources WHERE owner_id=$1 AND workspace_id=$2 AND id=$3`, owner, id, source))
}

const jobColumns = `id,workspace_id,source_id,company,role_title,job_description,confirmed_at`

func scanJob(row scanner) (intake.Revision, error) {
	var r intake.Revision
	err := row.Scan(&r.ID, &r.WorkspaceID, &r.SourceID, &r.Company, &r.RoleTitle, &r.Description, &r.ConfirmedAt)
	return r, notFound(err)
}
func (s FlowStore) ConfirmJob(ctx context.Context, owner, id string, in intake.Confirm) (intake.Confirmed, error) {
	var out intake.Confirmed
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	w, err := lockWorkspace(ctx, tx, owner, id, in.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if w.CurrentSourceID == nil || *w.CurrentSourceID != in.SourceID {
		return out, studio.ErrConflict
	}
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT source_version FROM job_sources WHERE owner_id=$1 AND workspace_id=$2 AND id=$3`, owner, id, in.SourceID).Scan(&version)
	if err != nil {
		return out, notFound(err)
	}
	if version != in.SourceVersion {
		return out, studio.ErrConflict
	}
	out.Revision, err = scanJob(tx.QueryRowContext(ctx, `INSERT INTO job_revisions(owner_id,workspace_id,source_id,company,role_title,job_description) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+jobColumns, owner, id, in.SourceID, in.Company, in.RoleTitle, in.Description))
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE workspaces SET job_revision_id=$3 WHERE owner_id=$1 AND id=$2 RETURNING version`, owner, id, out.Revision.ID).Scan(&out.WorkspaceVersion)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) JobRevision(ctx context.Context, owner, id, rev string) (intake.Revision, error) {
	return scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM job_revisions WHERE owner_id=$1 AND workspace_id=$2 AND id=$3`, owner, id, rev))
}
