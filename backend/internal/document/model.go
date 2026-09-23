package document

import (
	"applyflow/backend/internal/studio"
	"context"
	"time"
)

type EvidenceText struct {
	Text    string   `json:"text"`
	FactIDs []string `json:"fact_ids"`
}
type Section struct {
	Heading string         `json:"heading"`
	Items   []EvidenceText `json:"items"`
}

// Optional union members are omitted on output and rejected for the opposite kind.
type Content struct {
	Kind       string         `json:"kind"`
	Title      *string        `json:"title,omitempty"`
	Sections   []Section      `json:"sections,omitempty"`
	Salutation *string        `json:"salutation,omitempty"`
	Paragraphs []EvidenceText `json:"paragraphs,omitempty"`
	Closing    *string        `json:"closing,omitempty"`
}
type Document struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id"`
	Kind              string    `json:"kind"`
	CurrentRevisionID *string   `json:"current_revision_id"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type Revision struct {
	ID               string    `json:"id"`
	DocumentID       string    `json:"document_id"`
	Kind             string    `json:"kind"`
	ParentRevisionID *string   `json:"parent_revision_id"`
	RunID            string    `json:"run_id"`
	TaskID           *string   `json:"task_id"`
	Origin           string    `json:"origin"`
	SchemaVersion    int       `json:"schema_version"`
	Content          Content   `json:"content"`
	ChangeSummary    string    `json:"change_summary"`
	CreatedAt        time.Time `json:"created_at"`
}
type Save struct {
	ExpectedVersion int64   `json:"expected_version"`
	BaseRevisionID  string  `json:"base_revision_id"`
	Content         Content `json:"content"`
}
type Saved struct {
	Document Document `json:"document"`
	Revision Revision `json:"revision"`
}
type Apply struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
}
type Store interface {
	Documents(context.Context, string, string) ([]Document, error)
	Document(context.Context, string, string) (Document, error)
	DocumentRevision(context.Context, string, string, string) (Revision, error)
	Revisions(context.Context, string, string, int, *studio.Boundary) ([]Revision, error)
	SaveDocument(context.Context, string, string, Save) (Saved, error)
	ApplyDocument(context.Context, string, string, Apply) (Document, error)
}
