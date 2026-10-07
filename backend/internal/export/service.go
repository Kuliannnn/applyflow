// Package export renders immutable document revisions and commits private artifacts.
package export

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/task"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Create struct {
	RevisionID      string `json:"revision_id"`
	Format          string `json:"format"`
	TemplateVersion string `json:"template_version"`
}
type Export struct {
	ID              string    `json:"id"`
	DocumentID      string    `json:"document_id"`
	RevisionID      string    `json:"revision_id"`
	Format          string    `json:"format"`
	TemplateVersion string    `json:"template_version"`
	TaskID          string    `json:"task_id"`
	FileID          *string   `json:"file_id"`
	CreatedAt       time.Time `json:"created_at"`
}
type Accepted struct {
	Export Export
	Status int
}
type Store interface {
	CreateExport(context.Context, string, string, string, Create) (Accepted, error)
	Export(context.Context, string, string, string) (Export, error)
	LoadExport(context.Context, task.Claim) (Export, document.Revision, error)
	CompleteExport(context.Context, task.Claim, Export, files.File) error
}
type Renderer interface {
	Render(context.Context, string, string, document.Content) ([]byte, error)
}
type Blobs interface{ Put(string, []byte) error }
type Executor struct {
	Store    Store
	Renderer Renderer
	Blobs    Blobs
}

func (e Executor) Execute(ctx context.Context, c task.Claim) error {
	x, r, err := e.Store.LoadExport(ctx, c)
	if err != nil {
		return err
	}
	if err = document.Validate(r.Content, r.Kind, nil); err != nil {
		return err
	}
	raw, err := e.Renderer.Render(ctx, x.Format, x.TemplateVersion, r.Content)
	if err != nil {
		return err
	}
	if len(raw) == 0 || len(raw) > files.MaxBytes {
		return Error("export_size_limit")
	}
	digest := sha256.Sum256(raw)
	media := "application/pdf"
	if x.Format == "docx" {
		media = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	file := files.File{ID: security.UUID(), Purpose: "export", State: "ready", OriginalName: r.Kind + "-" + r.ID + "." + x.Format, MediaType: media, ByteSize: int64(len(raw)), SHA256: hex.EncodeToString(digest[:])}
	file.StorageKey = file.ID
	if err = e.Blobs.Put(file.StorageKey, raw); err != nil {
		return err
	}
	// Keep possible orphan bytes after an uncertain commit. Never delete a committed reference.
	return e.Store.CompleteExport(ctx, c, x, file)
}

type Error string

func (e Error) Error() string    { return string(e) }
func (e Error) SafeCode() string { return string(e) }
