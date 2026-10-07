package httpapi

import (
	"encoding/json"
	"time"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/resume"
	"github.com/gin-gonic/gin"
)

func (api *API) importResume(c *gin.Context) {
	if !api.allow(c, "resume-import:"+owner(c), 60, time.Minute) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	var in struct {
		FileID string `json:"file_id"`
		Name   string `json:"name"`
		Mode   string `json:"import_mode"`
	}
	if !decode(c, &in) {
		return
	}
	if !security.ValidUUID(key) || in.Mode != "manual" {
		problem(c, 400, "validation_error")
		return
	}
	r, err := api.options.Resumes.Import(c.Request.Context(), owner(c), in.FileID, in.Name, key)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Location", "/api/resumes/"+r.ID)
	respond(c, 201, gin.H{"resume": r, "task_id": nil})
}
func (api *API) getResume(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	r, err := api.options.Resumes.Get(c.Request.Context(), owner(c), id)
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, r)
}
func (api *API) patchResume(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64           `json:"expected_version"`
		Name            json.RawMessage `json:"name"`
		IsDefault       json.RawMessage `json:"is_default"`
	}
	if !decode(c, &in) {
		return
	}
	p := resume.Patch{ExpectedVersion: in.ExpectedVersion}
	if in.Name != nil {
		var v string
		if string(in.Name) == "null" || json.Unmarshal(in.Name, &v) != nil {
			problem(c, 400, "validation_error")
			return
		}
		p.Name = &v
	}
	if in.IsDefault != nil {
		var v bool
		if string(in.IsDefault) == "null" || json.Unmarshal(in.IsDefault, &v) != nil {
			problem(c, 400, "validation_error")
			return
		}
		p.IsDefault = &v
	}
	r, err := api.options.Resumes.Update(c.Request.Context(), owner(c), id, p)
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, r)
}
func (api *API) confirmResume(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
		Facts           []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Text     string `json:"text"`
			Evidence *struct {
				Source  string          `json:"source"`
				Page    json.RawMessage `json:"page"`
				Excerpt *string         `json:"excerpt"`
			} `json:"evidence"`
		} `json:"facts"`
	}
	if !decodeLimit(c, &in, 262144) {
		return
	}
	facts := make([]resume.Fact, 0, len(in.Facts))
	for _, f := range in.Facts {
		if f.Evidence == nil || f.Evidence.Page == nil || f.Evidence.Excerpt == nil {
			problem(c, 400, "validation_error")
			return
		}
		var page *int
		if json.Unmarshal(f.Evidence.Page, &page) != nil {
			problem(c, 400, "validation_error")
			return
		}
		facts = append(facts, resume.Fact{ID: f.ID, Category: f.Category, Text: f.Text, Evidence: resume.Evidence{Source: f.Evidence.Source, Page: page, Excerpt: *f.Evidence.Excerpt}})
	}
	result, err := api.options.Resumes.Confirm(c.Request.Context(), owner(c), id, in.ExpectedVersion, facts)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Location", "/api/resumes/"+id+"/revisions/"+result.Revision.ID)
	respond(c, 201, result)
}
func (api *API) getResumeRevision(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	rev := c.Param("revision_id")
	if !security.ValidUUID(rev) {
		problem(c, 400, "validation_error")
		return
	}
	r, err := api.options.Resumes.Revision(c.Request.Context(), owner(c), id, rev)
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, r)
}
func (api *API) listResumes(c *gin.Context) {
	limit, after, ok := api.listParameters(c, "resume-list-v1")
	if !ok {
		return
	}
	rows, err := api.options.Resumes.List(c.Request.Context(), owner(c), limit, after)
	if err != nil {
		api.failure(c, err)
		return
	}
	var next *string
	if len(rows) > limit {
		r := rows[limit-1]
		v := api.encodeCursor("resume-list-v1", owner(c), r.ID, r.UpdatedAt)
		next = &v
		rows = rows[:limit]
	}
	respond(c, 200, gin.H{"items": rows, "next_cursor": next})
}

// Source extraction is local and read-only. Confirmation remains an explicit,
// versioned write so parsing never silently replaces a user's confirmed resume.
func (api *API) extractResumeText(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if !api.allow(c, "resume-extract:"+owner(c), 10, time.Minute) {
		return
	}
	r, err := api.options.Resumes.Get(c.Request.Context(), owner(c), id)
	if err != nil {
		api.failure(c, err)
		return
	}
	if api.options.ResumeText == nil || api.options.Files == nil {
		problem(c, 503, "resume_extract_unavailable")
		return
	}
	select {
	case api.uploadSlots <- struct{}{}:
		defer func() { <-api.uploadSlots }()
	default:
		problem(c, 429, "rate_limited")
		return
	}
	f, data, err := api.options.Files.Download(c.Request.Context(), owner(c), r.SourceFileID)
	if err != nil {
		api.failure(c, err)
		return
	}
	out, err := api.options.ResumeText.Extract(c.Request.Context(), f.MediaType, data)
	if err != nil {
		switch err {
		case resume.ExtractionError("resume_ocr_required"):
			problem(c, 422, "resume_ocr_required")
		case resume.ExtractionError("resume_extract_too_large"):
			problem(c, 422, "resume_extract_too_large")
		default:
			problem(c, 422, "resume_extract_failed")
		}
		return
	}
	respond(c, 200, out)
}
