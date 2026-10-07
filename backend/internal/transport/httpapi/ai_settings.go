package httpapi

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"

	"applyflow/backend/internal/aisettings"

	"github.com/gin-gonic/gin"
)

func (api *API) aiProviders(c *gin.Context) {
	respond(c, 200, gin.H{"providers": aisettings.Providers(), "available": api.options.AI.Vault != nil, "platform_available": false, "generation_available": api.options.AI.Vault != nil, "test_notice": "Testing sends a short fixed prompt to the provider and may incur a small charge. No resume or job description is sent.", "test_limits": gin.H{"per_minute": 3, "per_day": 20}, "max_daily_request_limit": 100})
}
func aiRespond(c *gin.Context, status int, out aisettings.Config) {
	c.Header("ETag", `"ai-`+strconv.FormatInt(out.Version, 10)+`"`)
	respond(c, status, out)
}
func (api *API) getAI(c *gin.Context) {
	out, err := api.options.AI.Store.Get(c.Request.Context(), owner(c))
	if err != nil {
		api.aiFailure(c, err)
		return
	}
	aiRespond(c, 200, out)
}
func (api *API) putAI(c *gin.Context) {
	var body struct {
		ExpectedVersion   *int64          `json:"expected_version"`
		ProviderID        string          `json:"provider_id"`
		ModelID           string          `json:"model_id"`
		APIKey            json.RawMessage `json:"api_key"`
		DailyRequestLimit int             `json:"daily_request_limit"`
	}
	if !decodeLimit(c, &body, 8192) {
		return
	}
	if body.ExpectedVersion == nil {
		problem(c, 400, "validation_error")
		return
	}
	in := aisettings.Put{ExpectedVersion: *body.ExpectedVersion, ProviderID: body.ProviderID, ModelID: body.ModelID, DailyRequestLimit: body.DailyRequestLimit}
	if body.APIKey != nil {
		var key *string
		if json.Unmarshal(body.APIKey, &key) != nil || key == nil {
			problem(c, 400, "validation_error")
			return
		}
		in.APIKey = key
	}
	out, err := api.options.AI.Put(c.Request.Context(), owner(c), in)
	if err != nil {
		api.aiFailure(c, err)
		return
	}
	aiRespond(c, 200, out)
}
func (api *API) patchAI(c *gin.Context) {
	var body struct {
		ExpectedVersion *int64          `json:"expected_version"`
		Mode            json.RawMessage `json:"mode"`
		Enabled         json.RawMessage `json:"enabled"`
		Limit           json.RawMessage `json:"daily_request_limit"`
	}
	if !decodeLimit(c, &body, 2048) {
		return
	}
	if body.ExpectedVersion == nil {
		problem(c, 400, "validation_error")
		return
	}
	in := aisettings.Patch{ExpectedVersion: *body.ExpectedVersion}
	if body.Mode != nil && (json.Unmarshal(body.Mode, &in.Mode) != nil || in.Mode == nil) || body.Enabled != nil && (json.Unmarshal(body.Enabled, &in.Enabled) != nil || in.Enabled == nil) || body.Limit != nil && (json.Unmarshal(body.Limit, &in.DailyRequestLimit) != nil || in.DailyRequestLimit == nil) {
		problem(c, 400, "validation_error")
		return
	}
	out, err := api.options.AI.Patch(c.Request.Context(), owner(c), in)
	if err != nil {
		api.aiFailure(c, err)
		return
	}
	aiRespond(c, 200, out)
}
func (api *API) testAI(c *gin.Context) {
	var body struct {
		ExpectedVersion *int64 `json:"expected_version"`
		Revision        int64  `json:"revision"`
	}
	if !decodeLimit(c, &body, 1024) {
		return
	}
	if body.ExpectedVersion == nil {
		problem(c, 400, "validation_error")
		return
	}
	out, status, err := api.options.AI.Test(c.Request.Context(), owner(c), c.GetHeader("Idempotency-Key"), aisettings.Test{ExpectedVersion: *body.ExpectedVersion, Revision: body.Revision})
	if err != nil {
		api.aiFailure(c, err)
		return
	}
	aiRespond(c, status, out)
}

var aiETag = regexp.MustCompile(`^"ai-(0|[1-9][0-9]*)"$`)

func (api *API) deleteAI(c *gin.Context) {
	parts := aiETag.FindStringSubmatch(c.GetHeader("If-Match"))
	if parts == nil {
		problem(c, 400, "validation_error")
		return
	}
	version, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		problem(c, 400, "validation_error")
		return
	}
	out, err := api.options.AI.Store.Delete(c.Request.Context(), owner(c), version)
	if err != nil {
		api.aiFailure(c, err)
		return
	}
	aiRespond(c, 200, out)
}
func (api *API) aiFailure(c *gin.Context, err error) {
	code := ""
	status := 409
	switch {
	case errors.Is(err, aisettings.ErrInvalid):
		code = "validation_error"
		status = 400
	case errors.Is(err, aisettings.ErrConflict):
		code = "config_version_conflict"
	case errors.Is(err, aisettings.ErrTestRequired):
		code = "config_test_required"
	case errors.Is(err, aisettings.ErrChanged):
		code = "config_changed"
	case errors.Is(err, aisettings.ErrBusy):
		code = "config_test_in_progress"
	case errors.Is(err, aisettings.ErrKeyConflict):
		code = "idempotency_conflict"
	case errors.Is(err, aisettings.ErrRateLimited):
		code = "rate_limited"
		status = 429
		c.Header("Retry-After", "60")
	case errors.Is(err, aisettings.ErrUnavailable):
		code = "ai_credentials_unavailable"
		status = 503
	default:
		api.failure(c, err)
		return
	}
	problem(c, status, code)
}

func (api *API) aiUsage(c *gin.Context) {
	out, err := api.options.AI.Store.GenerationUsage(c.Request.Context(), owner(c))
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, out)
}
