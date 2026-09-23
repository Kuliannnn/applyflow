package studio

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid workspace input")
	ErrNotFound    = errors.New("workspace or source not found")
	ErrConflict    = errors.New("workspace version conflict")
	ErrArchived    = errors.New("workspace archived")
	ErrKeyConflict = errors.New("idempotency key reused")
)

type Workspace struct {
	ID, Title                                                                     string
	Version                                                                       int64
	CurrentSourceID, JobRevisionID, ResumeRevisionID, CurrentRunID, ApplicationID *string
	ArchivedAt                                                                    *time.Time
	CreatedAt, UpdatedAt                                                          time.Time
}
type Patch struct {
	ExpectedVersion  int64
	Title            *string
	SetResume        bool
	ResumeRevisionID *string
}
type Boundary struct {
	UpdatedAt time.Time
	ID        string
}
type Store interface {
	Create(context.Context, string, string, [32]byte, [32]byte) (Workspace, error)
	Get(context.Context, string, string) (Workspace, error)
	List(context.Context, string, int, *Boundary) ([]Workspace, error)
	Update(context.Context, string, string, Patch) (Workspace, error)
}
type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }
func validTitle(v string) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= 160 && strings.TrimSpace(v) != ""
}
func (s *Service) Create(ctx context.Context, owner, key string, title *string) (Workspace, error) {
	value := "Untitled application"
	if title != nil {
		value = *title
	}
	if !validTitle(value) {
		return Workspace{}, ErrInvalid
	}
	return s.store.Create(ctx, owner, value, sha256.Sum256([]byte(key)), sha256.Sum256([]byte(value)))
}
func (s *Service) Get(ctx context.Context, owner, id string) (Workspace, error) {
	return s.store.Get(ctx, owner, id)
}
func (s *Service) List(ctx context.Context, owner string, limit int, after *Boundary) ([]Workspace, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	return s.store.List(ctx, owner, limit+1, after)
}
func (s *Service) Update(ctx context.Context, owner, id string, p Patch) (Workspace, error) {
	if p.ExpectedVersion < 1 || p.ExpectedVersion > 9007199254740991 || (p.Title == nil && !p.SetResume) || (p.Title != nil && !validTitle(*p.Title)) {
		return Workspace{}, ErrInvalid
	}
	return s.store.Update(ctx, owner, id, p)
}
