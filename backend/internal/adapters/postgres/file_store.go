package postgres

import (
	"context"
	"database/sql"
	"errors"

	"applyflow/backend/internal/files"
	"applyflow/backend/internal/studio"
)

type FileStore struct{ DB *sql.DB }

const fileColumns = `id,purpose,state,original_name,media_type,byte_size,sha256,created_at,storage_key`

func scanFile(row scanner) (files.File, error) {
	var f files.File
	err := row.Scan(&f.ID, &f.Purpose, &f.State, &f.OriginalName, &f.MediaType, &f.ByteSize, &f.SHA256, &f.CreatedAt, &f.StorageKey)
	if errors.Is(err, sql.ErrNoRows) {
		err = studio.ErrNotFound
	}
	return f, err
}
func (s FileStore) Save(ctx context.Context, owner string, f files.File) (files.File, error) {
	return scanFile(s.DB.QueryRowContext(ctx, `INSERT INTO files(id,owner_id,purpose,state,original_name,media_type,byte_size,sha256,storage_key) VALUES($1,$2,$3,'ready',$4,$5,$6,$7,$8) RETURNING `+fileColumns, f.ID, owner, f.Purpose, f.OriginalName, f.MediaType, f.ByteSize, f.SHA256, f.StorageKey))
}
func (s FileStore) Get(ctx context.Context, owner, id string) (files.File, error) {
	return scanFile(s.DB.QueryRowContext(ctx, `SELECT `+fileColumns+` FROM files WHERE owner_id=$1 AND id=$2 AND state='ready'`, owner, id))
}
