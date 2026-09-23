package integration

import (
	"applyflow/backend/internal/task"
	"errors"
	"sync"
	"testing"
)

func TestWorkerClaimsRecoveryAndFencing(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	snapshots := f.generation(t, run.RunID)
	if snapshots.ResumeTask.Status != "queued" || string(snapshots.ResumeTask.Result) != "null" {
		t.Fatal("queued task must have null result")
	}
	claims := make([]*task.Claim, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range claims {
		wg.Add(1)
		go func(i int) { defer wg.Done(); claims[i], errs[i] = f.store.Claim(f.ctx) }(i)
	}
	wg.Wait()
	for i, c := range claims {
		if errs[i] != nil || c == nil {
			t.Fatalf("claim %d: %v", i, errs[i])
		}
	}
	if claims[0].ID == claims[1].ID {
		t.Fatal("workers claimed the same delivery")
	}
	if extra, err := f.store.Claim(f.ctx); err != nil || extra != nil {
		t.Fatal("running delivery reclaimed")
	}
	old := *claims[0]
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, old.ID)
	if err := f.executor().Execute(f.ctx, old); !errors.Is(err, task.ErrLeaseLost) {
		t.Fatalf("expired worker committed: %v", err)
	}
	if err := f.store.Recover(f.ctx); err != nil {
		t.Fatal(err)
	}
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE id=$1`, old.ID)
	if err := f.store.Recover(f.ctx); err != nil {
		t.Fatal(err)
	}
	next, err := f.store.Claim(f.ctx)
	if err != nil || next == nil {
		t.Fatal("task not recovered", err)
	}
	if next.ID != old.ID || next.Fence != old.Fence+1 {
		t.Fatal("recovery did not advance fence")
	}
	if err = f.executor().Execute(f.ctx, old); !errors.Is(err, task.ErrLeaseLost) {
		t.Fatalf("stale fence committed: %v", err)
	}
	if err = f.executor().Execute(f.ctx, *next); err != nil {
		t.Fatal(err)
	}
	if err = f.executor().Execute(f.ctx, *next); !errors.Is(err, task.ErrLeaseLost) {
		t.Fatal("duplicate delivery created a result")
	}
	if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM document_revisions WHERE task_id=$1`, next.ID); n != 1 {
		t.Fatal("duplicate revisions")
	}
	cancelled, err := f.store.Cancel(f.ctx, claims[1].OwnerID, claims[1].ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatal("running cancellation", err)
	}
	if err = f.executor().Execute(f.ctx, *claims[1]); !errors.Is(err, task.ErrLeaseLost) {
		t.Fatalf("cancelled worker committed: %v", err)
	}
	if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM document_revisions WHERE task_id=$1`, claims[1].ID); n != 0 {
		t.Fatal("cancelled revision persisted")
	}
}
func TestWorkerCrashBudgetAndResultRollback(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	// Cancel sibling to make the recovery schedule deterministic.
	requireStatus(t, f.a.request("POST", "/api/tasks/"+run.CoverLetterTaskID+"/cancel", "", nil), 200)
	for attempt := 1; attempt <= 3; attempt++ {
		c, err := f.store.Claim(f.ctx)
		if err != nil || c == nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID)
		if err = f.store.Recover(f.ctx); err != nil {
			t.Fatal(err)
		}
		if attempt < 3 {
			exec(t, f.ctx, f.db, `UPDATE job_tasks SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID)
			if err = f.store.Recover(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	g := f.generation(t, run.RunID)
	if g.ResumeTask.Status != "failed" || g.ResumeTask.Attempts != 3 || g.ResumeTask.ErrorCode == nil || *g.ResumeTask.ErrorCode != "attempts_exhausted" {
		t.Fatal("crashed job not terminal after budget")
	}
	// Retry a failed kind and inject a commit error after the task's completed update.
	retry := f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations/"+run.RunID+"/retry", `{"expected_version":5,"kind":"resume"}`, map[string]string{"Idempotency-Key": "90000000-0000-4000-8000-000000000001"})
	requireStatus(t, retry, 202)
	exec(t, f.ctx, f.db, `CREATE FUNCTION reject_mock_revision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic commit failure'; END; $$`)
	exec(t, f.ctx, f.db, `CREATE TRIGGER reject_mock_revision BEFORE INSERT ON document_revisions FOR EACH ROW EXECUTE FUNCTION reject_mock_revision()`)
	worked, err := (task.Worker{Store: f.store, Executor: f.executor()}).Tick(f.ctx)
	if !worked || err == nil {
		t.Fatal("expected execution failure")
	}
	g = f.generation(t, run.RunID)
	if g.ResumeTask.Status != "failed" || string(g.ResumeTask.Result) != "null" {
		t.Fatal("failed transaction left completed result")
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM document_revisions`) != 0 {
		t.Fatal("failed result transaction leaked revision")
	}
}
