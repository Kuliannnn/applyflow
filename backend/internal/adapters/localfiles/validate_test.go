package localfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/testfixture"
)

func TestPDFValidationFailureReasons(t *testing.T) {
	v, err := NewValidator("")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		data []byte
		want error
	}{
		{testfixture.PDF(21), files.ValidationError("pdf_page_limit")},
		{[]byte("%PDF-1.7\ninvalid"), files.ValidationError("invalid_pdf")},
	} {
		_, err := v.Validate(context.Background(), tc.data)
		if !errors.Is(err, tc.want) || !errors.Is(err, files.ErrUnsupported) {
			t.Fatalf("got %v, want %v", err, tc.want)
		}
	}
	missing := Validator{PDFInfo: filepath.Join(t.TempDir(), "missing-pdfinfo")}
	if _, err := missing.Validate(context.Background(), testfixture.PDF(1)); !errors.Is(err, files.ErrValidatorUnavailable) {
		t.Fatalf("execution failure misclassified: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.Validate(ctx, testfixture.PDF(1)); !errors.Is(err, files.ErrValidatorUnavailable) {
		t.Fatalf("cancelled validator misclassified: %v", err)
	}
}

func TestDocumentValidation(t *testing.T) {
	v, err := NewValidator("")
	if err != nil {
		t.Fatal("Poppler pdfinfo required:", err)
	}
	for name, data := range map[string][]byte{"pdf": testfixture.PDF(1), "docx": testfixture.DOCX(nil)} {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Validate(context.Background(), data); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, data := range map[string][]byte{
		"fake_pdf": []byte("%PDF-1.7\nnot a PDF"), "too_many_pages": testfixture.PDF(21), "plain_text": []byte("hello"),
		"macro":             testfixture.DOCX(map[string]string{"word/vbaProject.bin": "bad"}),
		"traversal":         testfixture.DOCX(map[string]string{"../bad": "bad"}),
		"external_template": testfixture.DOCX(map[string]string{"word/_rels/document.xml.rels": `<Relationships><Relationship Type="template" TargetMode="External" Target="https://example.com"/></Relationships>`}),
		"oversized_xml":     testfixture.DOCX(map[string]string{"word/document.xml": strings.Repeat("x", 8*1024*1024+1)}),
		"missing_document":  testfixture.DOCX(map[string]string{"word/document.xml": `<notword/>`}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Validate(context.Background(), data); err == nil {
				t.Fatal("unsafe document accepted")
			}
		})
	}
}
func TestPrivateStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := security.UUID()
	if err = s.Put(id, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err = s.Put(id, []byte("replacement")); err == nil {
		t.Fatal("file overwritten")
	}
	info, _ := os.Stat(filepath.Join(root, id))
	if info.Mode().Perm() != 0600 {
		t.Fatal("nonprivate file")
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err = os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink := security.UUID()
	if err = os.Symlink(outside, filepath.Join(root, symlink)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../secret", symlink} {
		if _, err = s.Read(key); err == nil {
			t.Fatal("read outside storage")
		}
	}
}
