package aiprovider

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestRejectedResponseLogsOnlySafeMetadata(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, status := range []int{401, 403, 404} {
		p := NewOpenAI()
		p.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html; private-header-secret"}}, Body: io.NopCloser(strings.NewReader("echoed-key-and-private-prompt")), Request: r}, nil
		})
		p.Test(context.Background(), "aiwanwu", "gpt-6-sol", []byte("private-test-key"))
	}
	logs := out.String()
	if !strings.Contains(logs, `"upstream_status":401`) || !strings.Contains(logs, `"upstream_status":403`) || !strings.Contains(logs, `"upstream_status":404`) || !strings.Contains(logs, `"response_format":"html"`) {
		t.Fatal("missing diagnostic metadata")
	}
	for _, secret := range []string{"private-header-secret", "echoed-key-and-private-prompt", "private-test-key"} {
		if strings.Contains(logs, secret) {
			t.Fatal("sensitive value in diagnostics")
		}
	}
}
