package aiprovider

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// Log only an allowlisted destination ID and response metadata, never provider
// bodies, request headers, keys or prompts. A gateway can echo secrets in errors.
func logRejectedResponse(ctx context.Context, provider string, response *http.Response) {
	format := "other"
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/json") {
		format = "json"
	}
	if strings.HasPrefix(contentType, "text/html") {
		format = "html"
	}
	slog.WarnContext(ctx, "AI provider rejected request", "provider", provider, "upstream_status", response.StatusCode, "response_format", format)
}
