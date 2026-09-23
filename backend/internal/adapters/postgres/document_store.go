package postgres

import (
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

const documentColumns = `id,workspace_id,kind,current_revision_id,version,created_at,updated_at`
const documentRevisionColumns = `id,document_id,kind,parent_revision_id,run_id,task_id,origin,schema_version,content,change_summary,created_at`

func scanDocument(row scanner) (document.Document, error) {
	var d document.Document
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.Kind, &d.CurrentRevisionID, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}
func scanDocumentRevision(row scanner) (document.Revision, error) {
	var r document.Revision
	var raw []byte
	err := row.Scan(&r.ID, &r.DocumentID, &r.Kind, &r.ParentRevisionID, &r.RunID, &r.TaskID, &r.Origin, &r.SchemaVersion, &raw, &r.ChangeSummary, &r.CreatedAt)
	if err == nil {
		err = json.Unmarshal(raw, &r.Content)
	}
	return r, notFound(err)
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryDocuments(ctx context.Context, q querier, owner, id string, lock bool) ([]document.Document, error) {
	query := `SELECT ` + documentColumns + ` FROM documents WHERE owner_id=$1 AND workspace_id=$2 ORDER BY id`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := q.QueryContext(ctx, query, owner, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]document.Document, 0)
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s FlowStore) Documents(ctx context.Context, owner, id string) ([]document.Document, error) {
	var exists bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE owner_id=$1 AND id=$2)`, owner, id).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, studio.ErrNotFound
	}
	return queryDocuments(ctx, s.DB, owner, id, false)
}
func (s FlowStore) Document(ctx context.Context, owner, id string) (document.Document, error) {
	return scanDocument(s.DB.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s FlowStore) DocumentRevision(ctx context.Context, owner, id, rev string) (document.Revision, error) {
	return scanDocumentRevision(s.DB.QueryRowContext(ctx, `SELECT `+documentRevisionColumns+` FROM document_revisions WHERE owner_id=$1 AND document_id=$2 AND id=$3`, owner, id, rev))
}
func (s FlowStore) Revisions(ctx context.Context, owner, id string, limit int, after *studio.Boundary) ([]document.Revision, error) {
	if _, err := s.Document(ctx, owner, id); err != nil {
		return nil, err
	}
	var stamp, boundary any
	if after != nil {
		stamp, boundary = after.UpdatedAt, after.ID
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+documentRevisionColumns+` FROM document_revisions WHERE owner_id=$1 AND document_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3::timestamptz,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, owner, id, stamp, boundary, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]document.Revision, 0)
	for rows.Next() {
		r, err := scanDocumentRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func lockDocument(ctx context.Context, tx *sql.Tx, owner, id string, version int64) (document.Document, studio.Workspace, error) {
	var d document.Document
	var w studio.Workspace
	if err := lockOwner(ctx, tx, owner); err != nil {
		return d, w, err
	}
	var workspace string
	err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM documents WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&workspace)
	if err != nil {
		return d, w, notFound(err)
	}
	w, err = scanWorkspace(tx.QueryRowContext(ctx, `SELECT `+workspaceColumns+` FROM workspaces WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, workspace))
	if err != nil {
		return d, w, err
	}
	if w.ArchivedAt != nil {
		return d, w, studio.ErrArchived
	}
	d, err = scanDocument(tx.QueryRowContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return d, w, err
	}
	if d.Version != version {
		return d, w, studio.ErrConflict
	}
	return d, w, nil
}
func runFacts(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, owner, run string) ([]resume.Fact, error) {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT rr.facts FROM generation_runs g JOIN resume_revisions rr ON rr.id=g.resume_revision_id AND rr.owner_id=g.owner_id WHERE g.owner_id=$1 AND g.id=$2`, owner, run).Scan(&raw)
	if err != nil {
		return nil, notFound(err)
	}
	var facts []resume.Fact
	err = json.Unmarshal(raw, &facts)
	return facts, err
}
func factSet(facts []resume.Fact) map[string]bool {
	out := map[string]bool{}
	for _, f := range facts {
		out[strings.ToLower(f.ID)] = true
	}
	return out
}
func (s FlowStore) SaveDocument(ctx context.Context, owner, id string, in document.Save) (document.Saved, error) {
	var out document.Saved
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	d, w, err := lockDocument(ctx, tx, owner, id, in.ExpectedVersion)
	if err != nil {
		return out, err
	}
	base, err := scanDocumentRevision(tx.QueryRowContext(ctx, `SELECT `+documentRevisionColumns+` FROM document_revisions WHERE owner_id=$1 AND document_id=$2 AND id=$3`, owner, id, in.BaseRevisionID))
	if err != nil {
		return out, err
	}
	if (d.CurrentRevisionID != nil && *d.CurrentRevisionID != base.ID) || (d.CurrentRevisionID == nil && (w.CurrentRunID == nil || base.RunID != *w.CurrentRunID)) {
		return out, studio.ErrConflict
	}
	facts, err := runFacts(ctx, tx, owner, base.RunID)
	if err != nil {
		return out, err
	}
	if err = document.Validate(in.Content, d.Kind, factSet(facts)); err != nil {
		return out, err
	}
	raw, err := json.Marshal(in.Content)
	if err != nil {
		return out, err
	}
	var size int
	if err = tx.QueryRowContext(ctx, `SELECT octet_length($1::jsonb::text)`, raw).Scan(&size); err != nil {
		return out, err
	}
	if size > 262144 {
		return out, studio.ErrInvalid
	}
	out.Revision, err = scanDocumentRevision(tx.QueryRowContext(ctx, `INSERT INTO document_revisions(owner_id,document_id,kind,parent_revision_id,run_id,origin,content,change_summary) VALUES($1,$2,$3,$4,$5,'manual',$6,'Manual edit') RETURNING `+documentRevisionColumns, owner, id, d.Kind, base.ID, base.RunID, raw))
	if err != nil {
		return out, err
	}
	out.Document, err = scanDocument(tx.QueryRowContext(ctx, `UPDATE documents SET current_revision_id=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+documentColumns, owner, id, out.Revision.ID))
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s FlowStore) ApplyDocument(ctx context.Context, owner, id string, in document.Apply) (document.Document, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return document.Document{}, err
	}
	defer tx.Rollback()
	d, w, err := lockDocument(ctx, tx, owner, id, in.ExpectedVersion)
	if err != nil {
		return d, err
	}
	r, err := scanDocumentRevision(tx.QueryRowContext(ctx, `SELECT `+documentRevisionColumns+` FROM document_revisions WHERE owner_id=$1 AND document_id=$2 AND id=$3`, owner, id, in.RevisionID))
	if err != nil {
		return d, err
	}
	if w.CurrentRunID == nil || r.RunID != *w.CurrentRunID {
		return d, studio.ErrConflict
	}
	d, err = scanDocument(tx.QueryRowContext(ctx, `UPDATE documents SET current_revision_id=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+documentColumns, owner, id, r.ID))
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
