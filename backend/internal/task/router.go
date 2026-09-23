package task

import (
	"context"
	"errors"
)

type Router map[string]Executor

func (r Router) Execute(ctx context.Context, c Claim) error {
	e, ok := r[c.Kind]
	if !ok {
		return errors.New("task executor unavailable")
	}
	return e.Execute(ctx, c)
}
