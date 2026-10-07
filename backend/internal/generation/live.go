package generation

import (
	"context"
	"time"

	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/platform/executionbudget"
	"applyflow/backend/internal/task"
)

// Failure is a stable, non-secret code suitable for HTTP and task responses.
type Failure string

func (f Failure) Error() string    { return string(f) }
func (f Failure) SafeCode() string { return string(f) }

type Credential struct {
	ID, Provider, Model string
	Sealed              aisettings.Sealed
}
type Usage struct{ InputTokens, OutputTokens *int64 }
type LiveResult struct {
	Content document.Content
	Usage   Usage
}
type LiveProvider interface {
	DraftLive(context.Context, string, Input, string, string, []byte) (LiveResult, error)
}
type LiveStore interface {
	BeginCall(context.Context, task.Claim) (Credential, error)
	FinishCall(context.Context, task.Claim, Usage, string) error
}

func (e Executor) live(ctx context.Context, c task.Claim, in Input, kind string) (document.Content, error) {
	if e.Live == nil || e.Vault == nil || e.Calls == nil {
		return document.Content{}, Failure("ai_credentials_unavailable")
	}
	credential, err := e.Calls.BeginCall(ctx, c)
	if err != nil {
		return document.Content{}, err
	}
	// BeginCall durably marks this task as dispatched before any external I/O.
	// If the process dies, recovery must never send this same task again.
	secret, err := e.Vault.Open(c.OwnerID, credential.ID, credential.Sealed)
	var result LiveResult
	if err != nil {
		err = Failure("credential_unavailable")
	} else {
		bounded, cancel := context.WithTimeout(ctx, executionbudget.ProviderCall)
		result, err = e.Live.DraftLive(bounded, kind, in, credential.Provider, credential.Model, secret)
		cancel()
		clear(secret)
	}
	state := "completed"
	if err != nil {
		state = "failed"
		if err == Failure("provider_timeout") || err == Failure("provider_unavailable") {
			state = "unknown"
		}
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if recordErr := e.Calls.FinishCall(finish, c, result.Usage, state); recordErr != nil {
		return document.Content{}, Failure("provider_outcome_unknown")
	}
	return result.Content, err
}
