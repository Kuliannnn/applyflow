package aiprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/resume"
)

func TestGeneratorStructuredOutputAndPrivacy(t *testing.T) {
	id := "20000000-0000-4000-8000-000000000001"
	input := generation.Input{Text: "Go engineer", Role: "Engineer", Company: "Example", Facts: []resume.Fact{{ID: id, Category: "experience", Text: "Built Go APIs", Evidence: resume.Evidence{Excerpt: "must-not-send-original-excerpt"}}}}
	for _, tc := range []struct{ name, kind, payload, status, want string }{
		{"resume", "resume", `{"sections":[{"heading":"Experience","items":[{"text":"Built Go APIs","fact_ids":["` + id + `"]}]}]}`, "completed", ""},
		{"letter", "cover_letter", `{"paragraphs":[{"text":"My experience includes building Go APIs.","fact_ids":["` + id + `"]}]}`, "completed", ""},
		{"unknown-evidence", "resume", `{"sections":[{"heading":"Experience","items":[{"text":"Invented","fact_ids":["20000000-0000-4000-8000-000000000002"]}]}]}`, "completed", "provider_invalid_document"},
		{"missing-evidence", "resume", `{"sections":[{"heading":"Experience","items":[{"text":"Invented","fact_ids":[]}]}]}`, "completed", "provider_invalid_document"},
		{"truncated", "resume", `{}`, "incomplete", "provider_incomplete"},
		{"extra-fields", "resume", `{"api_key":"bad","sections":[]}`, "completed", "provider_invalid_document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator()
			calls := 0
			g.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				if strings.Contains(string(raw), "must-not-send") || r.URL.String() != endpoint || r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Fatal("unsafe provider input")
				}
				var body map[string]any
				if json.Unmarshal(raw, &body) != nil || body["store"] != false || body["max_output_tokens"] != float64(4000) {
					t.Fatal("unbounded provider request")
				}
				envelope, _ := json.Marshal(map[string]any{"status": tc.status, "usage": map[string]int{"input_tokens": 100, "output_tokens": 50}, "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": tc.payload}}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope))), Header: http.Header{}, Request: r}, nil
			})
			out, err := g.DraftLive(context.Background(), tc.kind, input, "gpt-4.1-mini", []byte("synthetic-key"))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Fatalf("unexpected result %v", err)
			}
			if calls != 1 || out.Usage.InputTokens == nil || *out.Usage.InputTokens != 100 {
				t.Fatal("usage missing or repeated request")
			}
		})
	}
}
