package intake

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/studio"
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

type TextSource struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}
type Create struct {
	ExpectedVersion int64      `json:"expected_version"`
	Source          TextSource `json:"source"`
}
type Source struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Version     int64      `json:"source_version"`
	Source      TextSource `json:"source"`
	CreatedAt   time.Time  `json:"created_at"`
}
type Accepted struct {
	Source           Source `json:"source"`
	WorkspaceVersion int64  `json:"workspace_version"`
	TaskID           string `json:"task_id"`
}
type Confirm struct {
	ExpectedVersion int64  `json:"expected_version"`
	SourceID        string `json:"source_id"`
	SourceVersion   int64  `json:"expected_source_version"`
	Company         string `json:"company"`
	RoleTitle       string `json:"role_title"`
	Description     string `json:"job_description"`
}
type Revision struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	SourceID    string    `json:"source_id"`
	Company     string    `json:"company"`
	RoleTitle   string    `json:"role_title"`
	Description string    `json:"job_description"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
type Confirmed struct {
	Revision         Revision `json:"revision"`
	WorkspaceVersion int64    `json:"workspace_version"`
}
type Store interface {
	CreateSource(context.Context, string, string, string, Create) (Accepted, error)
	Source(context.Context, string, string, string) (Source, error)
	ConfirmJob(context.Context, string, string, Confirm) (Confirmed, error)
	JobRevision(context.Context, string, string, string) (Revision, error)
}
type Service struct{ Store Store }

func ValidText(v string, max int) bool {
	return utf8.ValidString(v) && !strings.ContainsRune(v, 0) && strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max
}
func ValidVersion(v int64) bool { return v >= 1 && v <= 9007199254740991 }
func (s Service) Create(ctx context.Context, owner, id, key string, in Create) (Accepted, error) {
	if !ValidVersion(in.ExpectedVersion) || !security.ValidUUID(key) || in.Source.Kind != "text" || !ValidText(in.Source.Text, 65536) || len(in.Source.Text) > 65536 {
		return Accepted{}, studio.ErrInvalid
	}
	return s.Store.CreateSource(ctx, owner, id, strings.ToLower(key), in)
}
func (s Service) Confirm(ctx context.Context, owner, id string, in Confirm) (Confirmed, error) {
	if !ValidVersion(in.ExpectedVersion) || !ValidVersion(in.SourceVersion) || !security.ValidUUID(in.SourceID) || !ValidText(in.Company, 200) || !ValidText(in.RoleTitle, 200) || !ValidText(in.Description, 65536) || len(in.Description) > 65536 {
		return Confirmed{}, studio.ErrInvalid
	}
	in.SourceID = strings.ToLower(in.SourceID)
	return s.Store.ConfirmJob(ctx, owner, id, in)
}
