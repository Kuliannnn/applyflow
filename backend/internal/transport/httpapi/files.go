package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"applyflow/backend/internal/files"
	"github.com/gin-gonic/gin"
)

func (api *API) uploadFile(c *gin.Context) {
	if !api.allow(c, "upload:"+owner(c), 20, time.Hour) {
		return
	}
	select {
	case api.uploadSlots <- struct{}{}:
		defer func() { <-api.uploadSlots }()
	default:
		c.Header("Retry-After", "2")
		problem(c, 429, "upload_busy")
		return
	}
	if c.Request.ContentLength > 11*1024*1024 {
		problem(c, 413, "payload_too_large")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11*1024*1024)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		problem(c, 415, "unsupported_media_type")
		return
	}
	seen := map[string]bool{}
	var purpose, name string
	var data []byte
	for {
		p, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			uploadReadError(c, err)
			return
		}
		field := p.FormName()
		if seen[field] || (field != "purpose" && field != "file") {
			problem(c, 400, "invalid_multipart")
			return
		}
		seen[field] = true
		// Parse raw filename ourselves; multipart.FileName strips paths silently.
		_, params, err := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if err != nil {
			problem(c, 400, "invalid_multipart")
			return
		}
		if field == "purpose" {
			if _, exists := params["filename"]; exists {
				problem(c, 400, "invalid_multipart")
				return
			}
			b, err := io.ReadAll(io.LimitReader(p, 65))
			if err != nil {
				uploadReadError(c, err)
				return
			}
			if len(b) > 64 {
				problem(c, 400, "validation_error")
				return
			}
			purpose = string(b)
		} else {
			name = params["filename"]
			data, err = io.ReadAll(io.LimitReader(p, files.MaxBytes+1))
			if err != nil {
				uploadReadError(c, err)
				return
			}
			if len(data) > files.MaxBytes {
				problem(c, 413, "payload_too_large")
				return
			}
		}
		_ = p.Close()
	}
	if !seen["file"] || !seen["purpose"] {
		problem(c, 400, "invalid_multipart")
		return
	}
	f, err := api.options.Files.Upload(c.Request.Context(), owner(c), purpose, name, data)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Location", "/api/files/"+f.ID+"/download")
	respond(c, 201, f)
}
func uploadReadError(c *gin.Context, err error) {
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		problem(c, 413, "payload_too_large")
	} else {
		problem(c, 400, "invalid_multipart")
	}
}
func (api *API) downloadFile(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	f, data, err := api.options.Files.Download(c.Request.Context(), owner(c), id)
	if err != nil {
		api.failure(c, err)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.OriginalName}))
	c.Data(200, "application/octet-stream", data)
}
