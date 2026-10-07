package aiprovider

import (
	"applyflow/backend/internal/aisettings"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestBoundedFixedDestinationProbe(t *testing.T) {
	success := `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}`
	for _, destination := range aisettings.Providers() {
		for _, tc := range []struct {
			name       string
			status     int
			body, want string
		}{
			{"ok", 200, success, "succeeded"}, {"auth", 401, "secret-from-provider", "failed"}, {"limit", 429, "", "failed"}, {"server", 503, "", "inconclusive"}, {"redirect", 302, "", "inconclusive"}, {"bad-json", 200, "oops", "inconclusive"}, {"empty", 200, `{"status":"completed","output":[]}`, "inconclusive"}, {"oversized", 200, strings.Repeat("x", maxResponseBytes+1), "inconclusive"},
		} {
			t.Run(destination.ID+"/"+tc.name, func(t *testing.T) {
				p := NewOpenAI()
				calls := 0
				p.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.String() != destination.Endpoint || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-only-secret" {
						t.Fatal("unsafe request")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["input"] != "Reply with OK." || body["model"] != destination.Models[0] || body["store"] != false || body["max_output_tokens"] != float64(32) {
						t.Fatal("unbounded or private probe")
					}
					return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://untrusted.invalid/"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
				})
				result := p.Test(context.Background(), destination.ID, destination.Models[0], []byte("test-only-secret"))
				if result.Status != tc.want || calls != 1 || strings.Contains(result.Code, "secret") {
					t.Fatalf("unexpected result %+v, calls %d", result, calls)
				}
			})
		}
	}
}
func TestProbeTimeoutIsInconclusive(t *testing.T) {
	p := NewOpenAI()
	p.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	r := p.Test(context.Background(), "openai", "gpt-4.1-mini", nil)
	if r.Status != "inconclusive" || r.Code != "provider_timeout" {
		t.Fatal(r)
	}
}

func TestUnregisteredDestinationNeverReceivesKey(t *testing.T) {
	for _, pair := range [][2]string{{"aiwanwu", "gpt-4.1"}, {"openai", "gpt-6-sol"}, {"https://localhost", "gpt-6-sol"}} {
		p := NewOpenAI()
		p.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network request"); return nil, nil })
		if got := p.Test(context.Background(), pair[0], pair[1], []byte("synthetic-key")); got.Code != "provider_model_unavailable" {
			t.Fatal(got)
		}
	}
}
