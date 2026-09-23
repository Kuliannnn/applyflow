package task

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Worker struct {
	Executor Executor
	Store    ExecutionStore
	Logger   *slog.Logger
}

// Tick is bounded and testable without timers. Each task commits independently.
func (w Worker) Tick(ctx context.Context) (bool, error) {
	if err := w.Store.Recover(ctx); err != nil {
		return false, err
	}
	claim, err := w.Store.Claim(ctx)
	if err != nil || claim == nil {
		return false, err
	}
	if err = w.Executor.Execute(ctx, *claim); err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return true, nil
		}
		if ctx.Err() != nil {
			return true, ctx.Err()
		} // scanner recovers interrupted work
		code := "mock_execution_failed"
		if claim.Kind == "export_document" {
			code = "export_failed"
		}
		var safe interface{ SafeCode() string }
		if errors.As(err, &safe) {
			code = safe.SafeCode()
		}
		if failErr := w.Store.Fail(ctx, *claim, code); failErr != nil {
			return true, failErr
		}
		return true, err
	}
	return true, nil
}
func (w Worker) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		worked, err := w.Tick(bounded)
		cancel()
		if err != nil && ctx.Err() == nil && w.Logger != nil {
			w.Logger.Error("worker iteration failed; durable recovery will retry")
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return nil
}
