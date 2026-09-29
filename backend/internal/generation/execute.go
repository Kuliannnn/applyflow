// Package generation validates provider output before a fenced result commit.
package generation

import (
	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
	"context"
	"strings"
)

type Input struct {
	Text, Role, Company string
	ExecutionMode       string
	Facts               []resume.Fact
}
type Store interface {
	LoadExecution(context.Context, task.Claim) (Input, error)
	CompleteExtraction(context.Context, task.Claim, string) error
	CompleteDocument(context.Context, task.Claim, document.Content) error
}
type Provider interface {
	Draft(string, string, string, []resume.Fact) document.Content
}
type Executor struct {
	Live     LiveProvider
	Vault    aisettings.Vault
	Calls    LiveStore
	Store    Store
	Provider Provider
}

func (e Executor) Execute(ctx context.Context, c task.Claim) error {
	in, err := e.Store.LoadExecution(ctx, c)
	if err != nil {
		return err
	}
	if c.Kind == "extract_jd" {
		if !intake.ValidText(in.Text, 65536) || len(in.Text) > 65536 {
			return studio.ErrInvalid
		}
		return e.Store.CompleteExtraction(ctx, c, in.Text)
	}
	kind := "resume"
	switch c.Kind {
	case "tailor_resume":
	case "write_cover_letter":
		kind = "cover_letter"
	default:
		return studio.ErrInvalid
	}
	var content document.Content
	if in.ExecutionMode == "personal" {
		content, err = e.live(ctx, c, in, kind)
		if err != nil {
			return err
		}
	} else {
		content = e.Provider.Draft(kind, in.Role, in.Company, in.Facts)
	}
	allowed := map[string]bool{}
	for _, f := range in.Facts {
		allowed[strings.ToLower(f.ID)] = true
	}
	if err = document.Validate(content, kind, allowed); err != nil {
		return Failure("provider_invalid_document")
	}
	return e.Store.CompleteDocument(ctx, c, content)
}
