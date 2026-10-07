package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"applyflow/backend/internal/adapters/credentialvault"
	"applyflow/backend/internal/adapters/provider"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/task"
)

type relayProbe struct {
	t     *testing.T
	calls int
}

func (p *relayProbe) Test(_ context.Context, provider, model string, key []byte) aisettings.ProbeResult {
	p.calls++
	if provider != "aiwanwu" || model != "gpt-6-sol" || string(key) != "relay-test-only-secret" {
		p.t.Fatal("wrong relay probe binding")
	}
	return aisettings.ProbeResult{Status: "succeeded"}
}
func TestProviderSwitchRequiresNewCredential(t *testing.T) {
	probe := &relayProbe{t: t}
	h, _, _, _ := newHTTPWithAI(t, probe)
	a := newBrowser(h)
	a.register(t, "relay@example.test")
	aiConfig(t, a.request("PUT", aiPath, aiSave, nil))
	body := `{"expected_version":1,"provider_id":"aiwanwu","model_id":"gpt-6-sol","daily_request_limit":20}`
	requireStatus(t, a.request("PUT", aiPath, body, nil), 400)
	body = strings.TrimSuffix(body, "}") + `,"api_key":"relay-test-only-secret"}`
	saved := aiConfig(t, a.request("PUT", aiPath, body, nil))
	if saved.Enabled || saved.TestStatus != "untested" || *saved.ProviderID != "aiwanwu" {
		t.Fatal(saved)
	}
	tested := aiConfig(t, a.request("POST", aiPath+"/test", fmt.Sprintf(`{"expected_version":%d,"revision":%d}`, saved.Version, *saved.Revision), map[string]string{"Idempotency-Key": security.UUID()}))
	if tested.TestStatus != "succeeded" || probe.calls != 1 {
		t.Fatal(tested, probe.calls)
	}
}

type relayLive struct {
	t     *testing.T
	calls int
}

func (p *relayLive) DraftLive(_ context.Context, kind string, in generation.Input, providerID, model string, key []byte) (generation.LiveResult, error) {
	p.calls++
	if providerID != "aiwanwu" || model != "gpt-6-sol" || string(key) != "test-only-secret-never-real" {
		p.t.Fatal("wrong durable relay binding")
	}
	return generation.LiveResult{Content: provider.Mock{}.Draft(kind, in.Role, in.Company, in.Facts)}, nil
}
func TestRelayGenerationRetainsProviderBinding(t *testing.T) {
	f := newFlow(t)
	body := strings.ReplaceAll(strings.ReplaceAll(aiSave, "openai", "aiwanwu"), "gpt-4.1-mini", "gpt-6-sol")
	aiConfig(t, f.a.request("PUT", aiPath, body, nil))
	aiConfig(t, f.a.request("POST", aiPath+"/test", `{"expected_version":1,"revision":1}`, map[string]string{"Idempotency-Key": security.UUID()}))
	aiConfig(t, f.a.request("PATCH", aiPath, `{"expected_version":3,"enabled":true}`, nil))
	accepted := acceptLive(t, &f)
	vault, _ := credentialvault.New([]byte(strings.Repeat("k", 32)), 1)
	live := &relayLive{t: t}
	worker := task.Worker{Store: f.store, Executor: generation.Executor{Store: f.store, Calls: f.store, Vault: vault, Live: live}}
	for i := 0; i < 2; i++ {
		if worked, err := worker.Tick(f.ctx); err != nil || !worked {
			t.Fatal(worked, err)
		}
	}
	result := f.generation(t, accepted.RunID)
	if live.calls != 2 || *result.ProviderID != "aiwanwu" || *result.ModelID != "gpt-6-sol" || result.ResumeTask.Status != "completed" || result.CoverLetterTask.Status != "completed" {
		t.Fatal(result, live.calls)
	}
}
