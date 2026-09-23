package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/studio"
	"github.com/gin-gonic/gin"
)

type workspaceDTO struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Version          int64      `json:"version"`
	CurrentSourceID  *string    `json:"current_source_id"`
	JobRevisionID    *string    `json:"job_revision_id"`
	ResumeRevisionID *string    `json:"resume_revision_id"`
	CurrentRunID     *string    `json:"current_run_id"`
	ApplicationID    *string    `json:"application_id"`
	ArchivedAt       *time.Time `json:"archived_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func workspace(w studio.Workspace) workspaceDTO {
	return workspaceDTO{w.ID, w.Title, w.Version, w.CurrentSourceID, w.JobRevisionID, w.ResumeRevisionID, w.CurrentRunID, w.ApplicationID, w.ArchivedAt, w.CreatedAt, w.UpdatedAt}
}
func pathID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !security.ValidUUID(id) {
		problem(c, 400, "validation_error")
		return "", false
	}
	return strings.ToLower(id), true
}
func (api *API) createWorkspace(c *gin.Context) {
	if !api.allow(c, "create:"+owner(c), 120, time.Minute) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if !security.ValidUUID(key) {
		problem(c, 400, "validation_error")
		return
	}
	var in struct {
		Title json.RawMessage `json:"title"`
	}
	if !decode(c, &in) {
		return
	}
	var title *string
	if in.Title != nil {
		var value string
		if json.Unmarshal(in.Title, &value) != nil || string(in.Title) == "null" {
			problem(c, 400, "validation_error")
			return
		}
		title = &value
	}
	w, err := api.studio.Create(c.Request.Context(), owner(c), strings.ToLower(key), title)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Location", "/api/workspaces/"+w.ID)
	respond(c, 201, workspace(w))
}
func (api *API) getWorkspace(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	w, err := api.studio.Get(c.Request.Context(), owner(c), id)
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, workspace(w))
}
func (api *API) patchWorkspace(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64           `json:"expected_version"`
		Title           json.RawMessage `json:"title"`
		Resume          json.RawMessage `json:"resume_revision_id"`
	}
	if !decode(c, &in) {
		return
	}
	p := studio.Patch{ExpectedVersion: in.ExpectedVersion, SetResume: in.Resume != nil}
	if in.Title != nil {
		var v string
		if json.Unmarshal(in.Title, &v) != nil || string(in.Title) == "null" {
			problem(c, 400, "validation_error")
			return
		}
		p.Title = &v
	}
	if in.Resume != nil && string(in.Resume) != "null" {
		var v string
		if json.Unmarshal(in.Resume, &v) != nil || !security.ValidUUID(v) {
			problem(c, 400, "validation_error")
			return
		}
		p.ResumeRevisionID = &v
	}
	w, err := api.studio.Update(c.Request.Context(), owner(c), id, p)
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, workspace(w))
}

type pageCursor struct {
	Owner     string    `json:"o"`
	ID        string    `json:"i"`
	UpdatedAt time.Time `json:"t"`
}

func (api *API) cursorSign(scope, v string) string {
	mac := hmac.New(sha256.New, api.options.CSRFKey)
	mac.Write([]byte(scope + ":" + v))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (api *API) cursor(owner string, w studio.Workspace) string {
	return api.encodeCursor("workspace-list-active-v1", owner, w.ID, w.UpdatedAt)
}
func (api *API) encodeCursor(scope, owner, id string, stamp time.Time) string {
	raw, _ := json.Marshal(pageCursor{owner, id, stamp})
	v := base64.RawURLEncoding.EncodeToString(raw)
	return v + "." + api.cursorSign(scope, v)
}
func (api *API) parseCursor(scope, owner, v string) (*studio.Boundary, error) {
	if len(v) > 2048 {
		return nil, studio.ErrInvalid
	}
	parts := strings.Split(v, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(api.cursorSign(scope, parts[0]))) {
		return nil, studio.ErrInvalid
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return nil, e
	}
	var p pageCursor
	if json.Unmarshal(raw, &p) != nil || p.Owner != owner || !security.ValidUUID(p.ID) || p.UpdatedAt.IsZero() {
		return nil, errors.New("invalid cursor")
	}
	return &studio.Boundary{UpdatedAt: p.UpdatedAt, ID: p.ID}, nil
}
func (api *API) listParameters(c *gin.Context, scope string) (int, *studio.Boundary, bool) {
	q := c.Request.URL.Query()
	for k, v := range q {
		if (k != "limit" && k != "cursor") || len(v) != 1 {
			problem(c, 400, "validation_error")
			return 0, nil, false
		}
	}
	limit := 20
	if _, exists := q["limit"]; exists {
		v, err := strconv.Atoi(q.Get("limit"))
		if err != nil || v < 1 || v > 100 {
			problem(c, 400, "validation_error")
			return 0, nil, false
		}
		limit = v
	}
	var after *studio.Boundary
	if _, exists := q["cursor"]; exists {
		var err error
		after, err = api.parseCursor(scope, owner(c), q.Get("cursor"))
		if err != nil {
			problem(c, 400, "invalid_cursor")
			return 0, nil, false
		}
	}
	return limit, after, true
}
func (api *API) listWorkspaces(c *gin.Context) {
	limit, after, ok := api.listParameters(c, "workspace-list-active-v1")
	if !ok {
		return
	}
	rows, err := api.studio.List(c.Request.Context(), owner(c), limit, after)
	if err != nil {
		api.failure(c, err)
		return
	}
	var next *string
	if len(rows) > limit {
		v := api.cursor(owner(c), rows[limit-1])
		next = &v
		rows = rows[:limit]
	}
	items := make([]workspaceDTO, 0, len(rows))
	for _, w := range rows {
		items = append(items, workspace(w))
	}
	respond(c, 200, gin.H{"items": items, "next_cursor": next})
}
