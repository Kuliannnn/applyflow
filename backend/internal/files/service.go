// Package files provides private, immutable source files; storage keys never enter responses.
package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/studio"
)

const MaxBytes = 10 * 1024 * 1024

var ErrUnsupported = errors.New("unsupported source file")
var ErrValidatorUnavailable = errors.New("document validator unavailable")

// ValidationError exposes only a stable reason, never document text or tool output.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }
func (e ValidationError) Unwrap() error { return ErrUnsupported }

type File struct {
	ID           string    `json:"id"`
	Purpose      string    `json:"purpose"`
	State        string    `json:"state"`
	OriginalName string    `json:"original_name"`
	MediaType    string    `json:"media_type"`
	ByteSize     int64     `json:"byte_size"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"created_at"`
	StorageKey   string    `json:"-"`
}
type Store interface {
	Save(context.Context, string, File) (File, error)
	Get(context.Context, string, string) (File, error)
}
type Blobs interface {
	Put(string, []byte) error
	Read(string) ([]byte, error)
}
type Validator interface {
	Validate(context.Context, []byte) (string, error)
}
type Service struct {
	store     Store
	blobs     Blobs
	validator Validator
}

func New(s Store, b Blobs, v Validator) *Service { return &Service{s, b, v} }
func (s *Service) Upload(ctx context.Context, owner, purpose, name string, data []byte) (File, error) {
	if purpose != "base_resume" || len(data) == 0 || len(data) > MaxBytes || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return File{}, studio.ErrInvalid
	}
	media, err := s.validator.Validate(ctx, data)
	if err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)
	f := File{ID: security.UUID(), Purpose: purpose, State: "ready", OriginalName: name, MediaType: media, ByteSize: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	f.StorageKey = f.ID
	if err = s.blobs.Put(f.StorageKey, data); err != nil {
		return File{}, err
	}
	// Do not delete a blob after an ambiguous DB commit/network error. A later orphan
	// sweep must prove it unreferenced; retaining an orphan is safer than losing a resume.
	return s.store.Save(ctx, owner, f)
}
func (s *Service) Download(ctx context.Context, owner, id string) (File, []byte, error) {
	f, err := s.store.Get(ctx, owner, id)
	if err != nil {
		return f, nil, err
	}
	b, err := s.blobs.Read(f.StorageKey)
	if err != nil {
		return f, nil, err
	}
	sum := sha256.Sum256(b)
	if int64(len(b)) != f.ByteSize || hex.EncodeToString(sum[:]) != f.SHA256 {
		return f, nil, errors.New("stored file integrity failure")
	}
	return f, b, nil
}
