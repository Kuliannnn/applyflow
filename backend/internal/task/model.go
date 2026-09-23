// Package task defines the durable execution boundary, independent of queue transport.
package task

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrLeaseLost = errors.New("task lease lost")

type Snapshot struct {
	ID              string          `json:"task_id"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"`
	Stage           string          `json:"stage"`
	StateVersion    int64           `json:"state_version"`
	Attempts        int             `json:"attempts"`
	CancelRequested bool            `json:"cancel_requested"`
	Result          json.RawMessage `json:"result"`
	ErrorCode       *string         `json:"error_code"`
	CreatedAt       time.Time       `json:"created_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
}
type Claim struct {
	ID, OwnerID, Kind, WorkspaceID string
	DocumentID, RunID, SourceID    *string
	Fence                          int64
	ExpectedVersion                *int64
}
type ReadStore interface {
	Task(context.Context, string, string) (Snapshot, error)
	Cancel(context.Context, string, string) (Snapshot, error)
}

// A transport claims work; results are validated and fenced by the execution store.
type ExecutionStore interface {
	Claim(context.Context) (*Claim, error)
	Recover(context.Context) error
	Fail(context.Context, Claim, string) error
}

type Executor interface {
	Execute(context.Context, Claim) error
}
