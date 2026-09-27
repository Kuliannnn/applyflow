package aisettings

import (
	"context"
	"strings"
	"time"

	"applyflow/backend/internal/adapters/security"
)

type Service struct {
	Store Store
	Vault Vault
	Probe Probe
}

func ValidVersion(v int64) bool { return v >= 0 && v <= 9007199254740991 }
func ValidModel(provider, model string) bool {
	return provider == "openai" && (model == "gpt-4.1-mini" || model == "gpt-4.1")
}
func (s *Service) Put(ctx context.Context, owner string, in Put) (Config, error) {
	if !ValidVersion(in.ExpectedVersion) || !ValidModel(in.ProviderID, in.ModelID) || in.DailyRequestLimit < 1 || in.DailyRequestLimit > 100 {
		return Config{}, ErrInvalid
	}
	if in.APIKey != nil {
		v := *in.APIKey
		if len(v) < 8 || len(v) > 4096 || strings.Contains(v, "***") {
			return Config{}, ErrInvalid
		}
		for _, c := range v {
			if c < 33 || c > 126 {
				return Config{}, ErrInvalid
			}
		}
	}
	if s.Vault == nil {
		return Config{}, ErrUnavailable
	}
	return s.Store.Put(ctx, owner, in, s.Vault)
}
func (s *Service) Patch(ctx context.Context, owner string, in Patch) (Config, error) {
	if !ValidVersion(in.ExpectedVersion) || (in.Mode == nil && in.Enabled == nil && in.DailyRequestLimit == nil) || (in.Mode != nil && *in.Mode != "personal") || (in.DailyRequestLimit != nil && (*in.DailyRequestLimit < 1 || *in.DailyRequestLimit > 100)) {
		return Config{}, ErrInvalid
	}
	if in.Enabled != nil && *in.Enabled && s.Vault == nil {
		return Config{}, ErrUnavailable
	}
	return s.Store.Patch(ctx, owner, in)
}
func (s *Service) Test(ctx context.Context, owner, key string, in Test) (Config, int, error) {
	if !ValidVersion(in.ExpectedVersion) || in.Revision < 1 || !ValidVersion(in.Revision) || !security.ValidUUID(key) {
		return Config{}, 0, ErrInvalid
	}
	if s.Vault == nil || s.Probe == nil {
		return Config{}, 0, ErrUnavailable
	}
	accepted, err := s.Store.StartTest(ctx, owner, strings.ToLower(key), in)
	if err != nil || accepted.Claim == nil {
		return accepted.Config, accepted.Status, err
	}
	claim := *accepted.Claim
	// The accepted test has its own bounded lifecycle. A disconnected client must
	// not cause the provider to be called again on an idempotent retry.
	call, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer cancel()
	result := ProbeResult{Status: "inconclusive", Code: "credential_unavailable"}
	secret, err := s.Vault.Open(owner, claim.CredentialID, claim.Sealed)
	if err == nil {
		result = s.Probe.Test(call, claim.ModelID, secret)
		clear(secret)
	}
	if result.Status != "succeeded" && result.Status != "failed" && result.Status != "inconclusive" {
		result = ProbeResult{Status: "inconclusive", Code: "provider_invalid_response"}
	}
	if result.Status == "succeeded" {
		result.Code = ""
	} else {
		switch result.Code {
		case "provider_auth_failed", "provider_model_unavailable", "provider_rate_limited", "provider_unavailable", "provider_invalid_response", "provider_timeout", "credential_unavailable":
		default:
			result.Code = "provider_unavailable"
		}
	}
	finish, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer done()
	out, err := s.Store.FinishTest(finish, claim, result)
	return out, 200, err
}
