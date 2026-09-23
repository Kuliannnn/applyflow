package studio

import (
	"applyflow/backend/internal/task"
	"context"
	"time"
)

type Generate struct {
	ExpectedVersion  int64  `json:"expected_version"`
	JobRevisionID    string `json:"job_revision_id"`
	ResumeRevisionID string `json:"resume_revision_id"`
	ProfileVersion   int64  `json:"expected_profile_version"`
	ExecutionMode    string `json:"execution_mode"`
	Locale           string `json:"locale"`
}
type GenerationAccepted struct {
	RunID             string `json:"run_id"`
	WorkspaceID       string `json:"workspace_id"`
	WorkspaceVersion  int64  `json:"workspace_version"`
	ResumeTaskID      string `json:"resume_task_id"`
	CoverLetterTaskID string `json:"cover_letter_task_id"`
}
type Generation struct {
	ID               string        `json:"id"`
	WorkspaceID      string        `json:"workspace_id"`
	JobRevisionID    string        `json:"job_revision_id"`
	ResumeRevisionID string        `json:"resume_revision_id"`
	ProfileVersion   int64         `json:"profile_version"`
	ExecutionMode    string        `json:"execution_mode"`
	Locale           string        `json:"locale"`
	ResumeTask       task.Snapshot `json:"resume_task"`
	CoverLetterTask  task.Snapshot `json:"cover_letter_task"`
	CreatedAt        time.Time     `json:"created_at"`
}
type Retry struct {
	ExpectedVersion int64  `json:"expected_version"`
	Kind            string `json:"kind"`
}
type TaskAccepted struct {
	TaskID string `json:"task_id"`
}
type GenerationStore interface {
	Generate(context.Context, string, string, string, Generate) (GenerationAccepted, error)
	Generation(context.Context, string, string, string) (Generation, error)
	RetryGeneration(context.Context, string, string, string, string, Retry) (TaskAccepted, error)
}
