package integration

import (
	"context"
	"strings"
	"testing"

	"applyflow/backend/internal/adapters/credentialvault"
	"applyflow/backend/internal/adapters/provider"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
)

type fakeLive func(context.Context, string, generation.Input, string, []byte) (generation.LiveResult, error)

func (f fakeLive) DraftLive(ctx context.Context, kind string, in generation.Input, model string, key []byte) (generation.LiveResult, error) {
	return f(ctx, kind, in, model, key)
}
func enableLive(t *testing.T, f flowFixture) {
	aiConfig(t, f.a.request("PUT", aiPath, aiSave, nil))
	aiConfig(t, f.a.request("POST", aiPath+"/test", `{"expected_version":1,"revision":1}`, map[string]string{"Idempotency-Key": security.UUID()}))
	aiConfig(t, f.a.request("PATCH", aiPath, `{"expected_version":3,"enabled":true}`, nil))
}
func acceptLive(t *testing.T, f *flowFixture) studio.GenerationAccepted {
	revision := int64(1)
	body := bodyJSON(t, studio.Generate{ExpectedVersion: f.version, JobRevisionID: f.job, ResumeRevisionID: f.revision, ProfileVersion: 1, ExecutionMode: "personal", AIRevision: &revision, Locale: "en"})
	key := security.UUID()
	w := f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", body, map[string]string{"Idempotency-Key": key})
	requireStatus(t, w, 202)
	repeat := f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", body, map[string]string{"Idempotency-Key": key})
	requireStatus(t, repeat, 202)
	if repeat.Body.String() != w.Body.String() {
		t.Fatal("acceptance replay changed")
	}
	out := decodeBody[studio.GenerationAccepted](t, w)
	f.version = out.WorkspaceVersion
	return out
}
func TestLiveGenerationAndDurableBudget(t *testing.T) {
	f := newFlow(t)
	enableLive(t, f)
	accepted := acceptLive(t, &f)
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM ai_generation_usage`) != 2 {
		t.Fatal("missing reservations")
	}
	vault, _ := credentialvault.New([]byte(strings.Repeat("k", 32)), 1)
	calls := 0
	live := fakeLive(func(_ context.Context, kind string, in generation.Input, model string, key []byte) (generation.LiveResult, error) {
		calls++
		if in.Text != "Build Go services." || string(key) != "test-only-secret-never-real" || model != "gpt-4.1-mini" {
			t.Fatal("wrong generation binding")
		}
		n := int64(100)
		return generation.LiveResult{Content: provider.Mock{}.Draft(kind, in.Role, in.Company, in.Facts), Usage: generation.Usage{InputTokens: &n, OutputTokens: &n}}, nil
	})
	worker := task.Worker{Store: f.store, Executor: generation.Executor{Store: f.store, Provider: provider.Mock{}, Calls: f.store, Vault: vault, Live: live}}
	for i := 0; i < 2; i++ {
		if worked, err := worker.Tick(f.ctx); err != nil || !worked {
			t.Fatal(worked, err)
		}
	}
	// Real-mode results retain the normal fixed-revision export path.
	ready := f.generation(t, accepted.RunID)
	docID, revisionID := resultIDs(t, ready.ResumeTask)
	for _, format := range []string{"pdf", "docx"} {
		path := "/api/documents/" + docID + "/exports"
		response := f.a.request("POST", path, exportInput(t, revisionID, format), map[string]string{"Idempotency-Key": security.UUID()})
		requireStatus(t, response, 202)
		artifact := decodeBody[export.Export](t, response)
		f.exportTick(t)
		finished := f.a.request("GET", path+"/"+artifact.ID, "", nil)
		requireStatus(t, finished, 200)
		value := decodeBody[export.Export](t, finished)
		if value.FileID == nil {
			t.Fatal("live draft export missing")
		}
		requireStatus(t, f.a.request("GET", "/api/files/"+*value.FileID+"/download", "", nil), 200)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	requireStatus(t, f.a.request("GET", "/api/me/ai-usage", "", nil), 200)
	run := decodeBody[studio.Generation](t, f.a.request("GET", "/api/workspaces/"+f.workspace+"/generations/"+accepted.RunID, "", nil))
	if run.ExecutionMode != "personal" || run.ResumeTask.Status != "completed" || run.ModelID == nil {
		t.Fatal(run)
	}
	if scalar(t, f.ctx, f.db, `SELECT sum(input_tokens) FROM ai_generation_usage`) != 200 {
		t.Fatal("usage not persisted")
	}
	exec(t, f.ctx, f.db, `UPDATE user_ai_settings SET daily_request_limit=2`)
	revision := int64(1)
	body := bodyJSON(t, studio.Generate{ExpectedVersion: f.version, JobRevisionID: f.job, ResumeRevisionID: f.revision, ProfileVersion: 1, ExecutionMode: "personal", AIRevision: &revision, Locale: "en"})
	requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", body, map[string]string{"Idempotency-Key": security.UUID()}), 429)
}
func TestLiveRecoveryNeverRepeatsDispatchedCall(t *testing.T) {
	f := newFlow(t)
	enableLive(t, f)
	acceptLive(t, &f)
	c, err := f.store.Claim(f.ctx)
	if err != nil || c == nil {
		t.Fatal(err)
	}
	if _, err = f.store.BeginCall(f.ctx, *c); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after dispatch, before a response can be durably recorded.
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID)
	if err = f.store.Recover(f.ctx); err != nil {
		t.Fatal(err)
	}
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID)
	if err = f.store.Recover(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Direct re-claim can pick either document; skip the other unsent task safely.
	for i := 0; i < 2; i++ {
		next, e := f.store.Claim(f.ctx)
		if e != nil || next == nil {
			t.Fatal(e)
		}
		if next.ID == c.ID {
			if _, e = f.store.BeginCall(f.ctx, *next); e != generation.Failure("provider_outcome_unknown") {
				t.Fatal("dispatched call permitted again", e)
			}
			return
		}
	}
	t.Fatal("recovered task not found")
}
func TestLiveDeletionStopsUnsentCalls(t *testing.T) {
	f := newFlow(t)
	enableLive(t, f)
	acceptLive(t, &f)
	aiConfig(t, f.a.request("DELETE", aiPath, "", map[string]string{"If-Match": `"ai-4"`}))
	c, err := f.store.Claim(f.ctx)
	if err != nil || c == nil {
		t.Fatal(err)
	}
	if _, err = f.store.BeginCall(f.ctx, *c); err != generation.Failure("ai_config_required") {
		t.Fatal("deleted credential dispatched", err)
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM ai_generation_usage WHERE state='started'`) != 0 {
		t.Fatal("charged deleted credential")
	}
}

func TestLivePartialFailureRequiresExplicitRetry(t *testing.T) {
	f := newFlow(t)
	enableLive(t, f)
	accepted := acceptLive(t, &f)
	vault, _ := credentialvault.New([]byte(strings.Repeat("k", 32)), 1)
	calls := 0
	failLetter := true
	live := fakeLive(func(_ context.Context, kind string, in generation.Input, _ string, _ []byte) (generation.LiveResult, error) {
		calls++
		if kind == "cover_letter" && failLetter {
			return generation.LiveResult{}, generation.Failure("provider_timeout")
		}
		return generation.LiveResult{Content: provider.Mock{}.Draft(kind, in.Role, in.Company, in.Facts)}, nil
	})
	worker := task.Worker{Store: f.store, Executor: generation.Executor{Store: f.store, Provider: provider.Mock{}, Calls: f.store, Vault: vault, Live: live}}
	for i := 0; i < 2; i++ {
		worker.Tick(f.ctx)
	}
	run := f.generation(t, accepted.RunID)
	if run.ResumeTask.Status != "completed" || run.CoverLetterTask.Status != "failed" {
		t.Fatal("partial success lost", run)
	}
	if worked, err := worker.Tick(f.ctx); worked || err != nil || calls != 2 {
		t.Fatal("automatic paid retry", worked, err, calls)
	}
	requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations/"+accepted.RunID+"/retry", bodyJSON(t, studio.Retry{ExpectedVersion: f.version, Kind: "cover_letter"}), map[string]string{"Idempotency-Key": security.UUID()}), 202)
	failLetter = false
	if worked, err := worker.Tick(f.ctx); !worked || err != nil || calls != 3 {
		t.Fatal(worked, err, calls)
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM ai_generation_usage WHERE state='unknown'`) != 1 {
		t.Fatal("uncertain usage lost")
	}
}

func TestLiveReservationsSerializeAndUsageIsPrivate(t *testing.T) {
	f := newFlow(t)
	enableLive(t, f)
	exec(t, f.ctx, f.db, `UPDATE user_ai_settings SET daily_request_limit=2`)
	var owner string
	if err := f.db.QueryRowContext(f.ctx, `SELECT id FROM users WHERE email='flow@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	revision := int64(1)
	input := studio.Generate{ExpectedVersion: f.version, JobRevisionID: f.job, ResumeRevisionID: f.revision, ProfileVersion: 1, ExecutionMode: "personal", AIRevision: &revision, Locale: "en"}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := f.store.Generate(f.ctx, owner, f.workspace, security.UUID(), input); results <- err }()
	}
	first, second := <-results, <-results
	if !((first == nil && second == generation.Failure("ai_daily_limit")) || (second == nil && first == generation.Failure("ai_daily_limit"))) {
		t.Fatal("concurrent budget admission", first, second)
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM ai_generation_usage`) != 2 {
		t.Fatal("quota overbooked")
	}
	other := f.b.request("GET", "/api/me/ai-usage", "", nil)
	requireStatus(t, other, 200)
	if !strings.Contains(other.Body.String(), `"reserved_requests":0`) {
		t.Fatal("cross-user usage exposed")
	}
	var id string
	if err := f.db.QueryRowContext(f.ctx, `SELECT task_id FROM ai_generation_usage LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Cancel(f.ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	usage := f.a.request("GET", "/api/me/ai-usage", "", nil)
	requireStatus(t, usage, 200)
	if !strings.Contains(usage.Body.String(), `"reserved_requests":1`) {
		t.Fatal("cancel did not release reservation")
	}
}
