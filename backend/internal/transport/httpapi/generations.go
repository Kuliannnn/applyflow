package httpapi

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/studio"
	"github.com/gin-gonic/gin"
	"strings"
	"time"
)

func (api *API) generate(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	if !api.allow(c, "generate:"+owner(c), 30, time.Minute) {
		return
	}
	var in studio.Generate
	if !decode(c, &in) {
		return
	}
	if !intake.ValidVersion(in.ExpectedVersion) || !intake.ValidVersion(in.ProfileVersion) || !security.ValidUUID(in.JobRevisionID) || !security.ValidUUID(in.ResumeRevisionID) || (in.ExecutionMode != "mock" && in.ExecutionMode != "personal") || in.Locale != "en" {
		problem(c, 400, "validation_error")
		return
	}
	if (in.ExecutionMode == "personal" && (in.AIRevision == nil || !intake.ValidVersion(*in.AIRevision))) || (in.ExecutionMode == "mock" && in.AIRevision != nil) {
		problem(c, 400, "validation_error")
		return
	}
	in.JobRevisionID = strings.ToLower(in.JobRevisionID)
	in.ResumeRevisionID = strings.ToLower(in.ResumeRevisionID)
	out, err := api.options.Generations.Generate(c.Request.Context(), owner(c), id, key, in)
	if err == nil {
		c.Header("Location", "/api/workspaces/"+id+"/generations/"+out.RunID)
	}
	api.result(c, 202, out, err)
}
func (api *API) getGeneration(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	run, ok := paramUUID(c, "run_id")
	if !ok {
		return
	}
	out, err := api.options.Generations.Generation(c.Request.Context(), owner(c), id, run)
	api.result(c, 200, out, err)
}
func (api *API) retryGeneration(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	run, ok := paramUUID(c, "run_id")
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	var in studio.Retry
	if !decode(c, &in) {
		return
	}
	if !intake.ValidVersion(in.ExpectedVersion) || (in.Kind != "resume" && in.Kind != "cover_letter") {
		problem(c, 400, "validation_error")
		return
	}
	out, err := api.options.Generations.RetryGeneration(c.Request.Context(), owner(c), id, run, key, in)
	if err == nil {
		c.Header("Location", "/api/tasks/"+out.TaskID)
	}
	api.result(c, 202, out, err)
}
func (api *API) getTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := api.options.Tasks.Task(c.Request.Context(), owner(c), id)
	api.result(c, 200, out, err)
}
func (api *API) cancelTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := api.options.Tasks.Cancel(c.Request.Context(), owner(c), id)
	api.result(c, 200, out, err)
}
