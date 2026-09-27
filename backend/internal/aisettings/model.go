// Package aisettings owns personal AI configuration and safe connection-test results.
package aisettings

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid      = errors.New("invalid AI settings")
	ErrConflict     = errors.New("AI configuration version conflict")
	ErrTestRequired = errors.New("AI configuration test required")
	ErrChanged      = errors.New("AI configuration changed")
	ErrBusy         = errors.New("AI test already running")
	ErrRateLimited  = errors.New("AI test rate limit")
	ErrKeyConflict  = errors.New("AI test idempotency conflict")
	ErrUnavailable  = errors.New("AI credential service unavailable")
)

type Config struct {
	Version           int64      `json:"version"`
	Mode              string     `json:"mode"`
	Enabled           bool       `json:"enabled"`
	Revision          *int64     `json:"revision"`
	ProviderID        *string    `json:"provider_id"`
	ModelID           *string    `json:"model_id"`
	HasKey            bool       `json:"has_key"`
	TestStatus        string     `json:"test_status"`
	LastTestedAt      *time.Time `json:"last_tested_at"`
	ErrorCode         *string    `json:"error_code"`
	DailyRequestLimit int        `json:"daily_request_limit"`
	PlatformAvailable bool       `json:"platform_available"`
}

func Empty() Config { return Config{Mode: "personal", TestStatus: "untested", DailyRequestLimit: 20} }

type Put struct {
	ExpectedVersion   int64   `json:"expected_version"`
	ProviderID        string  `json:"provider_id"`
	ModelID           string  `json:"model_id"`
	APIKey            *string `json:"api_key,omitempty"`
	DailyRequestLimit int     `json:"daily_request_limit"`
}
type Patch struct {
	ExpectedVersion   int64   `json:"expected_version"`
	Mode              *string `json:"mode,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	DailyRequestLimit *int    `json:"daily_request_limit,omitempty"`
}
type Test struct {
	ExpectedVersion int64 `json:"expected_version"`
	Revision        int64 `json:"revision"`
}
type Sealed struct {
	Ciphertext, Nonce []byte
	KeyVersion        int
}
type Vault interface {
	Seal(owner, id string, plaintext []byte) (Sealed, error)
	Open(owner, id string, sealed Sealed) ([]byte, error)
}
type Claim struct {
	OwnerID, RevisionID, RunID, CredentialID, ModelID string
	Sealed                                            Sealed
}
type Started struct {
	Config Config
	Status int
	Claim  *Claim
}
type ProbeResult struct{ Status, Code string }
type Probe interface {
	Test(context.Context, string, []byte) ProbeResult
}
type Store interface {
	Get(context.Context, string) (Config, error)
	Put(context.Context, string, Put, Vault) (Config, error)
	Patch(context.Context, string, Patch) (Config, error)
	Delete(context.Context, string, int64) (Config, error)
	StartTest(context.Context, string, string, Test) (Started, error)
	FinishTest(context.Context, Claim, ProbeResult) (Config, error)
}
