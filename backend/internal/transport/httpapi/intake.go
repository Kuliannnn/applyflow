package httpapi

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/intake"
	"github.com/gin-gonic/gin"
	"strings"
	"time"
)

func (api *API) result(c *gin.Context, status int, v any, err error) {
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, status, v)
}
func paramUUID(c *gin.Context, name string) (string, bool) {
	id := c.Param(name)
	if !security.ValidUUID(id) {
		problem(c, 400, "validation_error")
		return "", false
	}
	return strings.ToLower(id), true
}
func idempotencyKey(c *gin.Context) (string, bool) {
	key := c.GetHeader("Idempotency-Key")
	if !security.ValidUUID(key) {
		problem(c, 400, "validation_error")
		return "", false
	}
	return strings.ToLower(key), true
}
func (api *API) createSource(c *gin.Context) {
	if !api.allow(c, "source:"+owner(c), 60, time.Minute) {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	var in intake.Create
	if !decode(c, &in) {
		return
	}
	out, err := api.options.Intake.Create(c.Request.Context(), owner(c), id, key, in)
	if err == nil {
		c.Header("Location", "/api/tasks/"+out.TaskID)
	}
	api.result(c, 202, out, err)
}
func (api *API) getSource(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	source, ok := paramUUID(c, "source_id")
	if !ok {
		return
	}
	out, err := api.options.Intake.Store.Source(c.Request.Context(), owner(c), id, source)
	api.result(c, 200, out, err)
}
func (api *API) confirmJob(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in intake.Confirm
	if !decode(c, &in) {
		return
	}
	out, err := api.options.Intake.Confirm(c.Request.Context(), owner(c), id, in)
	if err == nil {
		c.Header("Location", "/api/workspaces/"+id+"/job-revisions/"+out.Revision.ID)
	}
	api.result(c, 201, out, err)
}
func (api *API) getJobRevision(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	rev, ok := paramUUID(c, "revision_id")
	if !ok {
		return
	}
	out, err := api.options.Intake.Store.JobRevision(c.Request.Context(), owner(c), id, rev)
	api.result(c, 200, out, err)
}
