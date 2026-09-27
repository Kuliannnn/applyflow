package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"applyflow/backend/internal/adapters/postgres"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
)

type testAIProbe func(context.Context, string, []byte) aisettings.ProbeResult

func (f testAIProbe) Test(ctx context.Context, model string, key []byte) aisettings.ProbeResult {
	return f(ctx, model, key)
}
func aiConfig(t *testing.T, w *httptest.ResponseRecorder) aisettings.Config {
	t.Helper()
	requireStatus(t, w, 200)
	var out aisettings.Config
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const aiPath = "/api/me/ai-config"
const aiSave = `{"expected_version":0,"provider_id":"openai","model_id":"gpt-4.1-mini","api_key":"test-only-secret-never-real","daily_request_limit":20}`

func TestAICredentialLifecycle(t *testing.T) {
	var calls atomic.Int32
	h, db, ctx, _ := newHTTPWithAI(t, testAIProbe(func(_ context.Context, model string, key []byte) aisettings.ProbeResult {
		calls.Add(1)
		if model != "gpt-4.1-mini" || string(key) != "test-only-secret-never-real" {
			t.Error("wrong probe inputs")
		}
		return aisettings.ProbeResult{Status: "succeeded"}
	}))
	a, b := newBrowser(h), newBrowser(h)
	requireStatus(t, a.request("GET", aiPath, "", nil), 401)
	a.register(t, "ai-alice@example.com")
	b.register(t, "ai-bob@example.com")
	out := aiConfig(t, a.request("GET", aiPath, "", nil))
	if out.Version != 0 || out.HasKey {
		t.Fatal(out)
	}
	requireStatus(t, a.request("PUT", aiPath, aiSave, map[string]string{"Origin": "https://elsewhere.invalid"}), 403)
	out = aiConfig(t, a.request("PUT", aiPath, aiSave, nil))
	if out.Version != 1 || out.Enabled || !out.HasKey || out.TestStatus != "untested" {
		t.Fatal(out)
	}
	requireStatus(t, a.request("PUT", aiPath, aiSave, nil), 409)
	requireStatus(t, a.request("PATCH", aiPath, `{"expected_version":1,"enabled":true}`, nil), 409)
	other := aiConfig(t, b.request("GET", aiPath, "", nil))
	if other.HasKey {
		t.Fatal("cross-account credential")
	}
	var cipher, nonce []byte
	if err := db.QueryRowContext(ctx, `SELECT ciphertext,nonce FROM ai_credentials`).Scan(&cipher, &nonce); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipher, []byte("test-only-secret-never-real")) || len(nonce) != 12 {
		t.Fatal("plaintext stored")
	}
	key := security.UUID()
	request := `{"expected_version":1,"revision":1}`
	tested := a.request("POST", aiPath+"/test", request, map[string]string{"Idempotency-Key": key})
	out = aiConfig(t, tested)
	if out.Version != 3 || out.TestStatus != "succeeded" || out.Enabled || calls.Load() != 1 {
		t.Fatal(out, calls.Load())
	}
	out = aiConfig(t, a.request("PATCH", aiPath, `{"expected_version":3,"enabled":true}`, nil))
	if !out.Enabled || out.Version != 4 {
		t.Fatal(out)
	}
	replay := a.request("POST", aiPath+"/test", request, map[string]string{"Idempotency-Key": key})
	requireStatus(t, replay, 200)
	if replay.Body.String() != tested.Body.String() || calls.Load() != 1 {
		t.Fatal("test replay repeated or changed")
	}
	requireStatus(t, a.request("POST", aiPath+"/test", `{"expected_version":4,"revision":1}`, map[string]string{"Idempotency-Key": key}), 409)
	out = aiConfig(t, a.request("PUT", aiPath, `{"expected_version":4,"provider_id":"openai","model_id":"gpt-4.1-mini","daily_request_limit":10}`, nil))
	if !out.Enabled || *out.Revision != 1 || out.TestStatus != "succeeded" {
		t.Fatal("limit invalidated test")
	}
	out = aiConfig(t, a.request("PUT", aiPath, `{"expected_version":5,"provider_id":"openai","model_id":"gpt-4.1","daily_request_limit":10}`, nil))
	if out.Enabled || *out.Revision != 2 || out.TestStatus != "untested" {
		t.Fatal("model did not invalidate test")
	}
	requireStatus(t, a.request("DELETE", aiPath, "", map[string]string{"If-Match": `"ai-5"`}), 409)
	out = aiConfig(t, a.request("DELETE", aiPath, "", map[string]string{"If-Match": `"ai-6"`}))
	if out.Version != 7 || out.HasKey || out.Revision != nil {
		t.Fatal(out)
	}
	if scalar(t, ctx, db, `SELECT count(*) FROM ai_credentials WHERE ciphertext IS NOT NULL OR nonce IS NOT NULL OR revoked_at IS NULL`) != 0 {
		t.Fatal("delete retained usable credential")
	}
	out = aiConfig(t, a.request("PUT", aiPath, strings.Replace(aiSave, `"expected_version":0`, `"expected_version":7`, 1), nil))
	if out.Version != 8 || *out.Revision != 3 {
		t.Fatal("delete reset counters")
	}
	var stored string
	if err := db.QueryRowContext(ctx, `SELECT safe_result::text FROM ai_test_requests`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{stored, tested.Body.String()} {
		if strings.Contains(raw, "test-only-secret") || strings.Contains(raw, "ciphertext") || strings.Contains(raw, "api_key") {
			t.Fatal("receipt exposed secret")
		}
	}
}
func TestAIStrictInput(t *testing.T) {
	h, _, _ := newHTTP(t)
	b := newBrowser(h)
	b.register(t, "ai-validation@example.com")
	for _, body := range []string{
		strings.Replace(aiSave, `"test-only-secret-never-real"`, `null`, 1), strings.Replace(aiSave, `"test-only-secret-never-real"`, `""`, 1),
		strings.Replace(aiSave, `"gpt-4.1-mini"`, `"unknown"`, 1), strings.Replace(aiSave, `"expected_version":0,`, "", 1),
		strings.Replace(aiSave, `"provider_id"`, `"Provider_ID"`, 1), strings.Replace(aiSave, `"daily_request_limit":20`, `"daily_request_limit":101`, 1),
		strings.Replace(aiSave, `"daily_request_limit":20`, `"base_url":"http://localhost:9000"`, 1),
	} {
		requireStatus(t, b.request("PUT", aiPath, body, nil), 400)
	}
	requireStatus(t, b.request("PATCH", aiPath, `{"expected_version":0,"enabled":null}`, nil), 400)
	requireStatus(t, b.request("DELETE", aiPath, "", nil), 400)
}
func TestAIInFlightReplayAndReplacementFence(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	h, db, ctx, _ := newHTTPWithAI(t, testAIProbe(func(ctx context.Context, _ string, _ []byte) aisettings.ProbeResult {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return aisettings.ProbeResult{Status: "succeeded"}
	}))
	b := newBrowser(h)
	b.register(t, "ai-race@example.com")
	aiConfig(t, b.request("PUT", aiPath, aiSave, nil))
	// Independent browser cookie maps avoid test-only map races.
	other := newBrowser(h)
	for k, v := range b.cookies {
		other.cookies[k] = v
	}
	other.csrf = b.csrf
	key := security.UUID()
	request := `{"expected_version":1,"revision":1}`
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- other.request("POST", aiPath+"/test", request, map[string]string{"Idempotency-Key": key})
	}()
	<-entered
	requireStatus(t, b.request("POST", aiPath+"/test", request, map[string]string{"Idempotency-Key": key}), 202)
	requireStatus(t, b.request("POST", aiPath+"/test", `{"expected_version":2,"revision":1}`, map[string]string{"Idempotency-Key": security.UUID()}), 409)
	replacement := strings.Replace(aiSave, `"expected_version":0`, `"expected_version":2`, 1)
	out := aiConfig(t, b.request("PUT", aiPath, replacement, nil))
	if *out.Revision != 2 {
		t.Fatal(out)
	}
	close(release)
	aiConfig(t, <-done)
	out = aiConfig(t, b.request("GET", aiPath, "", nil))
	if out.TestStatus != "untested" || out.Enabled || calls.Load() != 1 {
		t.Fatal("late success changed replacement")
	}
	var owner string
	if err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE email='ai-race@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	store := postgres.AISettingsStore{DB: db}
	started, err := store.StartTest(ctx, owner, security.UUID(), aisettings.Test{ExpectedVersion: out.Version, Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE ai_test_requests SET deadline=now()-interval '1 second' WHERE run_id=$1`, started.Claim.RunID); err != nil {
		t.Fatal(err)
	}
	out, err = store.Get(ctx, owner)
	if err != nil || out.TestStatus != "inconclusive" || out.ErrorCode == nil || *out.ErrorCode != "test_interrupted" {
		t.Fatal(out, err)
	}
	if _, err = store.FinishTest(ctx, *started.Claim, aisettings.ProbeResult{Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	out, err = store.Get(ctx, owner)
	if err != nil || out.TestStatus != "inconclusive" {
		t.Fatal("expired test revived")
	}
}
func TestAITestBudgetAndDeleteFence(t *testing.T) {
	h, db, ctx := newHTTP(t)
	b := newBrowser(h)
	b.register(t, "ai-budget@example.com")
	out := aiConfig(t, b.request("PUT", aiPath, aiSave, nil))
	for i := 0; i < 3; i++ {
		out = aiConfig(t, b.request("POST", aiPath+"/test", fmt.Sprintf(`{"expected_version":%d,"revision":1}`, out.Version), map[string]string{"Idempotency-Key": security.UUID()}))
	}
	requireStatus(t, b.request("POST", aiPath+"/test", fmt.Sprintf(`{"expected_version":%d,"revision":1}`, out.Version), map[string]string{"Idempotency-Key": security.UUID()}), 429)
	if _, err := db.ExecContext(ctx, `UPDATE ai_test_requests SET created_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE email='ai-budget@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	store := postgres.AISettingsStore{DB: db}
	started, err := store.StartTest(ctx, owner, security.UUID(), aisettings.Test{ExpectedVersion: out.Version, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.Delete(ctx, owner, started.Config.Version)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := store.FinishTest(ctx, *started.Claim, aisettings.ProbeResult{Status: "succeeded"})
	if err != nil || finished.Version != deleted.Version || finished.HasKey || finished.Enabled {
		t.Fatal("delete fence failed", err)
	}
}
