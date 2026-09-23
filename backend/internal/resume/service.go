// Package resume owns confirmed facts. Import is explicitly manual until a parser worker exists.
package resume

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/studio"
)

type Resume struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	SourceFileID      string    `json:"source_file_id"`
	CurrentRevisionID *string   `json:"current_revision_id"`
	IsDefault         bool      `json:"is_default"`
	Version           int64     `json:"version"`
	LatestTaskID      *string   `json:"latest_task_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type Evidence struct {
	Source  string `json:"source"`
	Page    *int   `json:"page"`
	Excerpt string `json:"excerpt"`
}
type Fact struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Text     string   `json:"text"`
	Evidence Evidence `json:"evidence"`
}
type Revision struct {
	ID               string    `json:"id"`
	ResumeID         string    `json:"resume_id"`
	ParentRevisionID *string   `json:"parent_revision_id"`
	SchemaVersion    int       `json:"schema_version"`
	Facts            []Fact    `json:"facts"`
	ConfirmedAt      time.Time `json:"confirmed_at"`
}
type Confirmed struct {
	Revision      Revision `json:"revision"`
	ResumeVersion int64    `json:"resume_version"`
}
type Patch struct {
	ExpectedVersion int64
	Name            *string
	IsDefault       *bool
}
type Store interface {
	Import(context.Context, string, string, string, [32]byte, [32]byte) (Resume, error)
	Get(context.Context, string, string) (Resume, error)
	List(context.Context, string, int, *studio.Boundary) ([]Resume, error)
	Update(context.Context, string, string, Patch) (Resume, error)
	Confirm(context.Context, string, string, int64, []Fact) (Confirmed, error)
	Revision(context.Context, string, string, string) (Revision, error)
}
type Service struct{ store Store }

func New(s Store) *Service { return &Service{store: s} }
func validText(s string, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
}
func validVersion(v int64) bool { return v >= 1 && v <= 9007199254740991 }
func (s *Service) Import(ctx context.Context, owner, file, name, key string) (Resume, error) {
	if !security.ValidUUID(file) || !security.ValidUUID(key) || !validText(name, 160) {
		return Resume{}, studio.ErrInvalid
	}
	file = strings.ToLower(file)
	raw, _ := json.Marshal([]string{file, name, "manual"})
	return s.store.Import(ctx, owner, file, name, sha256.Sum256([]byte(strings.ToLower(key))), sha256.Sum256(raw))
}
func (s *Service) Get(ctx context.Context, owner, id string) (Resume, error) {
	return s.store.Get(ctx, owner, id)
}
func (s *Service) List(ctx context.Context, owner string, limit int, b *studio.Boundary) ([]Resume, error) {
	if limit < 1 || limit > 100 {
		return nil, studio.ErrInvalid
	}
	return s.store.List(ctx, owner, limit+1, b)
}
func (s *Service) Update(ctx context.Context, owner, id string, p Patch) (Resume, error) {
	if !validVersion(p.ExpectedVersion) || (p.Name == nil && p.IsDefault == nil) || (p.Name != nil && !validText(*p.Name, 160)) {
		return Resume{}, studio.ErrInvalid
	}
	return s.store.Update(ctx, owner, id, p)
}
func (s *Service) Confirm(ctx context.Context, owner, id string, version int64, facts []Fact) (Confirmed, error) {
	if !validVersion(version) || len(facts) < 1 || len(facts) > 200 {
		return Confirmed{}, studio.ErrInvalid
	}
	seen := map[string]bool{}
	for i := range facts {
		f := &facts[i]
		f.ID = strings.ToLower(f.ID)
		if !security.ValidUUID(f.ID) || seen[f.ID] || !validText(f.Text, 4000) {
			return Confirmed{}, studio.ErrInvalid
		}
		seen[f.ID] = true
		switch f.Category {
		case "summary", "experience", "education", "skill", "project", "other":
		default:
			return Confirmed{}, studio.ErrInvalid
		}
		e := f.Evidence
		if (e.Source != "file" && e.Source != "user") || !utf8.ValidString(e.Excerpt) || strings.ContainsRune(e.Excerpt, 0) || utf8.RuneCountInString(e.Excerpt) > 4000 || (e.Page != nil && (*e.Page < 1 || *e.Page > 20)) {
			return Confirmed{}, studio.ErrInvalid
		}
	}
	return s.store.Confirm(ctx, owner, id, version, facts)
}
func (s *Service) Revision(ctx context.Context, owner, id, rev string) (Revision, error) {
	return s.store.Revision(ctx, owner, id, rev)
}
