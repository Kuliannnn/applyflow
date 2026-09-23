package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
)

type ResumeStore struct{ DB *sql.DB }

const resumeColumns = `id,name,source_file_id,current_revision_id,is_default,version,created_at,updated_at`

func scanResume(row scanner) (resume.Resume, error) {
	var r resume.Resume
	err := row.Scan(&r.ID, &r.Name, &r.SourceFileID, &r.CurrentRevisionID, &r.IsDefault, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = studio.ErrNotFound
	}
	return r, err
}
func lockOwner(ctx context.Context, tx *sql.Tx, owner string) error {
	var id string
	return tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&id)
}
func (s ResumeStore) Import(ctx context.Context, owner, file, name string, key, hash [32]byte) (resume.Resume, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return resume.Resume{}, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return resume.Resume{}, err
	}
	kh, rh := hex.EncodeToString(key[:]), hex.EncodeToString(hash[:])
	var prior string
	var body []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_requests WHERE owner_id=$1 AND operation='import_resume_manual' AND key_hash=$2`, owner, kh).Scan(&prior, &body)
	if err == nil {
		if prior != rh {
			return resume.Resume{}, studio.ErrKeyConflict
		}
		var r resume.Resume
		if err = json.Unmarshal(body, &r); err != nil {
			return r, err
		}
		return r, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return resume.Resume{}, err
	}
	var found bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM files WHERE id=$1 AND owner_id=$2 AND state='ready' AND purpose='base_resume')`, file, owner).Scan(&found); err != nil {
		return resume.Resume{}, err
	}
	if !found {
		return resume.Resume{}, studio.ErrNotFound
	}
	r, err := scanResume(tx.QueryRowContext(ctx, `INSERT INTO resumes(owner_id,name,source_file_id) VALUES($1,$2,$3) RETURNING `+resumeColumns, owner, name, file))
	if err != nil {
		return r, err
	}
	body, err = json.Marshal(r)
	if err != nil {
		return r, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES($1,'import_resume_manual',$2,$3,201,$4,now()+interval '24 hours')`, owner, kh, rh, body)
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func (s ResumeStore) Get(ctx context.Context, owner, id string) (resume.Resume, error) {
	return scanResume(s.DB.QueryRowContext(ctx, `SELECT `+resumeColumns+` FROM resumes WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s ResumeStore) List(ctx context.Context, owner string, limit int, after *studio.Boundary) ([]resume.Resume, error) {
	var stamp, id any
	if after != nil {
		stamp, id = after.UpdatedAt, after.ID
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+resumeColumns+` FROM resumes WHERE owner_id=$1 AND ($2::timestamptz IS NULL OR (updated_at,id)<($2::timestamptz,$3::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $4`, owner, stamp, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]resume.Resume, 0)
	for rows.Next() {
		r, err := scanResume(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (s ResumeStore) Update(ctx context.Context, owner, id string, p resume.Patch) (resume.Resume, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return resume.Resume{}, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return resume.Resume{}, err
	}
	r, err := scanResume(tx.QueryRowContext(ctx, `SELECT `+resumeColumns+` FROM resumes WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return r, err
	}
	if r.Version != p.ExpectedVersion {
		return r, studio.ErrConflict
	}
	if p.IsDefault != nil && *p.IsDefault {
		if r.CurrentRevisionID == nil {
			return r, studio.ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE resumes SET is_default=false WHERE owner_id=$1 AND is_default AND id<>$2`, owner, id); err != nil {
			return r, err
		}
	}
	r, err = scanResume(tx.QueryRowContext(ctx, `UPDATE resumes SET name=COALESCE($4,name),is_default=COALESCE($5,is_default) WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING `+resumeColumns, owner, id, p.ExpectedVersion, p.Name, p.IsDefault))
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}

const revisionColumns = `id,resume_id,parent_revision_id,schema_version,facts,confirmed_at`

func scanResumeRevision(row scanner) (resume.Revision, error) {
	var r resume.Revision
	var raw []byte
	err := row.Scan(&r.ID, &r.ResumeID, &r.ParentRevisionID, &r.SchemaVersion, &raw, &r.ConfirmedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = studio.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &r.Facts)
	}
	return r, err
}
func (s ResumeStore) Confirm(ctx context.Context, owner, id string, version int64, facts []resume.Fact) (resume.Confirmed, error) {
	var out resume.Confirmed
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = lockOwner(ctx, tx, owner); err != nil {
		return out, err
	}
	r, err := scanResume(tx.QueryRowContext(ctx, `SELECT `+resumeColumns+` FROM resumes WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if err != nil {
		return out, err
	}
	if r.Version != version {
		return out, studio.ErrConflict
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return out, err
	}
	// Check PostgreSQL's JSONB representation (including whitespace) before its constraint.
	var size int
	if err = tx.QueryRowContext(ctx, `SELECT octet_length($1::jsonb::text)`, raw).Scan(&size); err != nil {
		return out, err
	}
	if size > 262144 {
		return out, studio.ErrInvalid
	}
	out.Revision, err = scanResumeRevision(tx.QueryRowContext(ctx, `INSERT INTO resume_revisions(owner_id,resume_id,parent_revision_id,facts) VALUES($1,$2,$3,$4) RETURNING `+revisionColumns, owner, id, r.CurrentRevisionID, raw))
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `UPDATE resumes SET current_revision_id=$4 WHERE owner_id=$1 AND id=$2 AND version=$3 RETURNING version`, owner, id, version, out.Revision.ID).Scan(&out.ResumeVersion)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s ResumeStore) Revision(ctx context.Context, owner, id, rev string) (resume.Revision, error) {
	return scanResumeRevision(s.DB.QueryRowContext(ctx, `SELECT `+revisionColumns+` FROM resume_revisions WHERE owner_id=$1 AND resume_id=$2 AND id=$3`, owner, id, rev))
}
