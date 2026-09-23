package httpapi

import (
	"applyflow/backend/internal/files"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestFileValidationErrorsAreActionable(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{files.ValidationError("invalid_pdf"), 415, "invalid_pdf"},
		{files.ValidationError("pdf_restricted"), 415, "pdf_restricted"},
		{files.ValidationError("pdf_page_limit"), 415, "pdf_page_limit"},
		{files.ErrUnsupported, 415, "unsupported_source_file"},
		{files.ErrValidatorUnavailable, 503, "file_validator_unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			(&API{}).failure(c, tc.err)
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Code != tc.code || c.GetString("error_code") != tc.code {
				t.Fatalf("unexpected response/log classification: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
