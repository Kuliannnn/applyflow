package localfiles

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/testfixture"
)

func TestResumeTextExtractionPreservesWholeSource(t *testing.T) {
	validator, err := NewValidator("")
	if err != nil {
		t.Fatal(err)
	}
	e := NewTextExtractor(validator.PDFInfo)
	text := "Alex Example — alex@example.test\nEDUCATION: Bachelor of Computing, 2024\nEXPERIENCE: Built Go APIs and reduced batch processing time by 20 percent.\n" + strings.Repeat("Project details with dates and measurable results.\n", 160)
	xml := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`
	out, err := e.Extract(context.Background(), docxMIME, testfixture.DOCX(map[string]string{"word/document.xml": xml}))
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	for _, f := range out.Facts {
		if utf8.RuneCountInString(f.Text) > 3500 || f.Evidence.Source != "file" || f.Evidence.Excerpt != f.Text {
			t.Fatal("invalid extracted chunk")
		}
		joined.WriteString(f.Text)
	}
	if strings.ReplaceAll(joined.String(), "\n", "") != strings.ReplaceAll(strings.TrimSpace(text), "\n", "") {
		t.Fatal("source text was lost")
	}
	if _, err := e.Extract(context.Background(), "application/pdf", testfixture.PDF(1)); err != resume.ExtractionError("resume_ocr_required") {
		t.Fatal("blank/scanned PDF must not yield confirmed facts", err)
	}
}
