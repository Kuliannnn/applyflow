package httpapi

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/intake"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"reflect"
	"strings"
)

func (api *API) listDocuments(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := api.options.Documents.Documents(c.Request.Context(), owner(c), id)
	api.result(c, 200, gin.H{"items": out}, err)
}
func (api *API) getDocument(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := api.options.Documents.Document(c.Request.Context(), owner(c), id)
	api.result(c, 200, out, err)
}
func (api *API) getDocumentRevision(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	rev, ok := paramUUID(c, "revision_id")
	if !ok {
		return
	}
	out, err := api.options.Documents.DocumentRevision(c.Request.Context(), owner(c), id, rev)
	api.result(c, 200, out, err)
}
func (api *API) listDocumentRevisions(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	scope := "document-revisions-v1:" + id
	limit, after, ok := api.listParameters(c, scope)
	if !ok {
		return
	}
	out, err := api.options.Documents.Revisions(c.Request.Context(), owner(c), id, limit+1, after)
	if err != nil {
		api.failure(c, err)
		return
	}
	var next *string
	if len(out) > limit {
		last := out[limit-1]
		v := api.encodeCursor(scope, owner(c), last.ID, last.CreatedAt)
		next = &v
		out = out[:limit]
	}
	respond(c, 200, gin.H{"items": out, "next_cursor": next})
}
func (api *API) saveDocument(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64           `json:"expected_version"`
		BaseRevisionID  string          `json:"base_revision_id"`
		Content         json.RawMessage `json:"content"`
	}
	if !decodeLimit(c, &in, 262144) {
		return
	}
	if !intake.ValidVersion(in.ExpectedVersion) || !security.ValidUUID(in.BaseRevisionID) {
		problem(c, 400, "validation_error")
		return
	}
	var content document.Content
	if exactShape(in.Content, reflect.TypeOf(content)) != nil || json.Unmarshal(in.Content, &content) != nil {
		problem(c, 400, "validation_error")
		return
	}
	// Enforce the union even for explicit null members from the opposite kind.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(in.Content, &fields)
	allowed := map[string]bool{"kind": true, "title": true, "sections": true}
	if content.Kind == "cover_letter" {
		allowed = map[string]bool{"kind": true, "salutation": true, "paragraphs": true, "closing": true}
	}
	if len(fields) != len(allowed) {
		problem(c, 400, "validation_error")
		return
	}
	for field := range fields {
		if !allowed[field] {
			problem(c, 400, "validation_error")
			return
		}
	}
	if err := document.Validate(content, content.Kind, nil); err != nil {
		api.failure(c, err)
		return
	}
	out, err := api.options.Documents.SaveDocument(c.Request.Context(), owner(c), id, document.Save{ExpectedVersion: in.ExpectedVersion, BaseRevisionID: strings.ToLower(in.BaseRevisionID), Content: content})
	if err == nil {
		c.Header("Location", "/api/documents/"+id+"/revisions/"+out.Revision.ID)
	}
	api.result(c, 201, out, err)
}
func (api *API) applyDocument(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in document.Apply
	if !decode(c, &in) {
		return
	}
	if !intake.ValidVersion(in.ExpectedVersion) || !security.ValidUUID(in.RevisionID) {
		problem(c, 400, "validation_error")
		return
	}
	in.RevisionID = strings.ToLower(in.RevisionID)
	out, err := api.options.Documents.ApplyDocument(c.Request.Context(), owner(c), id, in)
	api.result(c, 200, out, err)
}
