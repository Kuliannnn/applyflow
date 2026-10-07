// Package aiprovider contains bounded, fixed-destination connection probes.
package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"applyflow/backend/internal/aisettings"
)

const maxResponseBytes = 64 * 1024

type OpenAI struct{ client *http.Client }

func NewOpenAI() *OpenAI {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 3 * time.Second
	transport.ResponseHeaderTimeout = 5 * time.Second
	transport.MaxResponseHeaderBytes = 16 * 1024
	return &OpenAI{client: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *OpenAI) Test(ctx context.Context, provider, model string, secret []byte) aisettings.ProbeResult {
	failed := func(status, code string) aisettings.ProbeResult {
		return aisettings.ProbeResult{Status: status, Code: code}
	}
	destination, ok := aisettings.ResolveProvider(provider, model)
	if !ok {
		return failed("failed", "provider_model_unavailable")
	}
	body, _ := json.Marshal(map[string]any{"model": model, "input": "Reply with OK.", "max_output_tokens": 32, "store": false, "stream": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destination.Endpoint, bytes.NewReader(body))
	if err != nil {
		return failed("inconclusive", "provider_unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+string(secret))
	req.Header.Set("Content-Type", "application/json")
	// No idempotency header or automatic application retry: an uncertain paid call
	// must remain uncertain until the user explicitly starts another test.
	resp, err := p.client.Do(req)
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			return failed("inconclusive", "provider_timeout")
		}
		return failed("inconclusive", "provider_unavailable")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401, 403:
		logRejectedResponse(ctx, provider, resp)
		return failed("failed", "provider_auth_failed")
	case 404:
		logRejectedResponse(ctx, provider, resp)
		return failed("failed", "provider_model_unavailable")
	case 429:
		return failed("failed", "provider_rate_limited")
	case 200:
	default:
		if resp.StatusCode >= 500 || resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return failed("inconclusive", "provider_unavailable")
		}
		return failed("failed", "provider_invalid_response")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return failed("inconclusive", "provider_invalid_response")
	}
	var result struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Status != "completed" {
		return failed("inconclusive", "provider_invalid_response")
	}
	for _, item := range result.Output {
		for _, content := range item.Content {
			if item.Type == "message" && content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return aisettings.ProbeResult{Status: "succeeded"}
			}
		}
	}
	return failed("inconclusive", "provider_invalid_response")
}
