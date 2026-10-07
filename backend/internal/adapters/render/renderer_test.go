package render

import (
	"applyflow/backend/internal/adapters/localfiles"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/export"
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(kind string) document.Content {
	if kind == "letter" {
		salutation, closing := "Dear Hiring Team,", "Sincerely,\nAlex Morgan"
		return document.Content{Kind: "cover_letter", Salutation: &salutation, Closing: &closing, Paragraphs: []document.EvidenceText{
			{Text: "I am applying for the Backend Engineer role. My experience includes building Go services and maintaining PostgreSQL-backed applications.", FactIDs: []string{}},
			{Text: "I worked with product and engineering colleagues to define API contracts, improve error handling, and document deployment procedures. I also added integration tests for concurrent updates and recovery behavior.", FactIDs: []string{}},
			{Text: "I would welcome an opportunity to discuss how this experience relates to your team's priorities. Thank you for considering my application.", FactIDs: []string{}},
		}}
	}
	title := "Alex Morgan Resume"
	return document.Content{Kind: "resume", Title: &title, Sections: []document.Section{
		{Heading: "Profile", Items: []document.EvidenceText{{Text: "Backend engineer working with Go, PostgreSQL, and reliable API design. Based in Sydney, Australia.", FactIDs: []string{}}}},
		{Heading: "Experience", Items: []document.EvidenceText{{Text: "Backend Engineer | Northstar | 2022 - 2025\nBuilt and maintained API services. Added transaction tests, reviewed database migrations, and documented operating procedures.", FactIDs: []string{}}, {Text: "Collaborated with engineers and designers to simplify the application workflow and preserve user edits during background processing.", FactIDs: []string{}}}},
		{Heading: "Skills", Items: []document.EvidenceText{{Text: "Go, PostgreSQL, TypeScript, HTTP APIs, automated testing, and technical documentation.", FactIDs: []string{}}}},
		{Heading: "Education", Items: []document.EvidenceText{{Text: "Bachelor of Computer Science | Example University | 2021", FactIDs: []string{}}}},
	}}
}
func TestRenderFormatsAndBoundaries(t *testing.T) {
	r, err := New(context.Background(), os.Getenv("EXPORT_PYTHON"))
	if err != nil {
		t.Fatal(err)
	}
	long := fixture("resume")
	long.Sections = []document.Section{{Heading: "Long experience entry", Items: []document.EvidenceText{{Text: strings.Repeat("Designed services with clear API contracts and transaction boundaries. Tested recovery behavior and documented the operational requirements. ", 26), FactIDs: []string{}}}}, {Heading: "Additional project", Items: []document.EvidenceText{{Text: strings.Repeat("Maintained applications with versioned data and reliable background processing. ", 15), FactIDs: []string{}}}}}
	samples := map[string]document.Content{"resume": fixture("resume"), "letter": fixture("letter"), "long-resume": long}
	for name, content := range samples {
		for _, format := range []string{"pdf", "docx"} {
			t.Run(name+"-"+format, func(t *testing.T) {
				raw, err := r.Render(context.Background(), format, "2", content)
				if err == nil && format == "pdf" && name == "resume" {
					validator, e := localfiles.NewValidator("")
					if e != nil {
						t.Fatal(e)
					}
					source, e := localfiles.NewTextExtractor(validator.PDFInfo).Extract(context.Background(), "application/pdf", raw)
					if e != nil {
						t.Fatal("exported PDF cannot be read back", e)
					}
					var full strings.Builder
					for _, fact := range source.Facts {
						full.WriteString(fact.Text)
						full.WriteString(" ")
					}
					normalized := strings.Join(strings.Fields(strings.ReplaceAll(full.String(), "•", "")), " ")
					for _, section := range content.Sections {
						for _, item := range section.Items {
							if !strings.Contains(normalized, strings.Join(strings.Fields(item.Text), " ")) {
								t.Fatal("PDF extraction lost source detail")
							}
						}
					}
				}

				if err != nil {
					t.Fatal(err)
				}
				if format == "pdf" {
					if !bytes.HasPrefix(raw, []byte("%PDF-")) || !bytes.Contains(raw, []byte("%%EOF")) {
						t.Fatal("invalid PDF")
					}
				} else {
					z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, f := range z.File {
						if f.Name == "word/styles.xml" {
							p, err := f.Open()
							if err != nil {
								t.Fatal(err)
							}
							b, err := io.ReadAll(p)
							p.Close()
							if err != nil {
								t.Fatal(err)
							}
							if bytes.Contains(b, []byte("<w:pBdr")) {
								t.Fatal("inherited decorative paragraph border")
							}
						}
						if f.Name == "word/document.xml" {
							p, err := f.Open()
							if err != nil {
								t.Fatal(err)
							}
							b, err := io.ReadAll(p)
							p.Close()
							if err != nil {
								t.Fatal(err)
							}
							if !bytes.Contains(b, []byte("<w:t")) {
								t.Fatal("DOCX is not editable text")
							}
							found = true
						}
					}
					if !found {
						t.Fatal("missing document part")
					}
				}
				if dir := os.Getenv("EXPORT_QA_DIR"); dir != "" {
					if err = os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(dir, name+"."+format), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
	bad := fixture("resume")
	title := "Unsupported 😀"
	bad.Title = &title
	if _, err = r.Render(context.Background(), "pdf", "2", bad); !errors.Is(err, export.Error("export_unsupported_character")) {
		t.Fatalf("unsupported glyph silently substituted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.Render(ctx, "pdf", "2", fixture("resume")); err == nil {
		t.Fatal("cancelled renderer succeeded")
	}
	escaped := fixture("resume")
	literal := "Literal <script> & URL https://example.com/test"
	escaped.Title = &literal
	raw, err := r.Render(context.Background(), "pdf", "2", escaped)
	if err != nil || len(raw) == 0 {
		t.Fatal("plain text markup interpreted", err)
	}
}
