// Package executionbudget keeps provider, worker and database lease deadlines
// aligned. A lease must outlive execution so another worker cannot reclaim a
// task while its bounded provider call is still allowed to run.
package executionbudget

import "time"

const (
	ProviderCall  = 3 * time.Minute
	TaskExecution = ProviderCall + 15*time.Second
	TaskLease     = TaskExecution + 15*time.Second
)
