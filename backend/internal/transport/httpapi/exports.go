package httpapi

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/export"
	"github.com/gin-gonic/gin"
	"strings"
	"time"
)

func (api *API) createExport(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	if !api.allow(c, "export:"+owner(c), 30, time.Minute) {
		return
	}
	var in export.Create
	if !decode(c, &in) {
		return
	}
	if !security.ValidUUID(in.RevisionID) || (in.Format != "pdf" && in.Format != "docx") || (in.TemplateVersion != "1" && in.TemplateVersion != "2") {
		problem(c, 400, "validation_error")
		return
	}
	in.RevisionID = strings.ToLower(in.RevisionID)
	out, err := api.options.Exports.CreateExport(c.Request.Context(), owner(c), id, key, in)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Location", "/api/documents/"+id+"/exports/"+out.Export.ID)
	respond(c, out.Status, out.Export)
}
func (api *API) getExport(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	x, ok := paramUUID(c, "export_id")
	if !ok {
		return
	}
	out, err := api.options.Exports.Export(c.Request.Context(), owner(c), id, x)
	api.result(c, 200, out, err)
}
