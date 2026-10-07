package localfiles

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/resume"
)

type TextExtractor struct{ PDFToText string }

func NewTextExtractor(pdfInfo string) TextExtractor {
	// Prefer the configured Poppler installation, then support separately installed
	// utilities (including environments that wrap pdfinfo). Never use client paths.
	path := filepath.Join(filepath.Dir(pdfInfo), "pdftotext")
	if _, err := exec.LookPath(path); err != nil {
		if resolved, lookupErr := exec.LookPath("pdftotext"); lookupErr == nil {
			path = resolved
		}
	}
	return TextExtractor{PDFToText: path}
}

type extractBuffer struct{ bytes.Buffer }

func (b *extractBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 256*1024 {
		return 0, errors.New("extraction output limit")
	}
	return b.Buffer.Write(p)
}
func (e TextExtractor) Extract(ctx context.Context, media string, data []byte) (resume.Extraction, error) {
	fail := func(code string) (resume.Extraction, error) { return resume.Extraction{}, resume.ExtractionError(code) }
	if len(data) == 0 || len(data) > files.MaxBytes {
		return fail("resume_extract_failed")
	}
	var pages []string
	switch media {
	case "application/pdf":
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, e.PDFToText, "-layout", "-enc", "UTF-8", "-", "-")
		cmd.Env = []string{"LC_ALL=C", "LANG=C"}
		cmd.Stdin = bytes.NewReader(data)
		out := &extractBuffer{}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return fail("resume_extract_failed")
		}
		pages = strings.Split(strings.TrimSuffix(out.String(), "\f"), "\f")
	case docxMIME:
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fail("resume_extract_failed")
		}
		var text strings.Builder
		sort.SliceStable(z.File, func(i, j int) bool {
			rank := func(name string) int {
				if strings.HasPrefix(name, "word/header") {
					return 0
				}
				if name == "word/document.xml" {
					return 1
				}
				return 2
			}
			return rank(z.File[i].Name) < rank(z.File[j].Name)
		})
		for _, f := range z.File {
			if f.Name != "word/document.xml" && !(strings.HasPrefix(f.Name, "word/header") && strings.HasSuffix(f.Name, ".xml")) && !(strings.HasPrefix(f.Name, "word/footer") && strings.HasSuffix(f.Name, ".xml")) {
				continue
			}
			if f.UncompressedSize64 > 2*1024*1024 {
				return fail("resume_extract_too_large")
			}
			r, err := f.Open()
			if err != nil {
				return fail("resume_extract_failed")
			}
			decoder := xml.NewDecoder(io.LimitReader(r, 2*1024*1024+1))
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					r.Close()
					return fail("resume_extract_failed")
				}
				switch t := token.(type) {
				case xml.StartElement:
					switch t.Name.Local {
					case "t":
						var value string
						if err := decoder.DecodeElement(&value, &t); err != nil {
							r.Close()
							return fail("resume_extract_failed")
						}
						text.WriteString(value)
					case "tab":
						text.WriteString("\t")
					case "br":
						text.WriteString("\n")
					}
				case xml.EndElement:
					if t.Name.Local == "p" {
						text.WriteString("\n")
					}
				}
				if text.Len() > 48000 {
					r.Close()
					return fail("resume_extract_too_large")
				}
			}
			r.Close()
		}
		pages = []string{text.String()}
	default:
		return fail("resume_extract_failed")
	}
	if len(pages) > 20 {
		return fail("resume_extract_too_large")
	}
	result := resume.Extraction{PageCount: len(pages), Facts: []resume.Fact{}}
	total := 0
	for i, page := range pages {
		if !utf8.ValidString(page) {
			return fail("resume_extract_failed")
		}
		page = strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, page))
		// Never silently confirm a mixed scanned/text document with missing pages.
		if len([]rune(page)) < 20 {
			return fail("resume_ocr_required")
		}
		total += len(page)
		if total > 48000 {
			return fail("resume_extract_too_large")
		}
		remaining := []rune(page)
		for len(remaining) > 0 {
			end := len(remaining)
			if end > 3500 {
				end = 3500
				for j := end; j > 2000; j-- {
					if remaining[j-1] == '\n' {
						end = j
						break
					}
				}
			}
			chunk := strings.TrimSpace(string(remaining[:end]))
			remaining = remaining[end:]
			if chunk == "" {
				continue
			}
			var pageNumber *int
			if media == "application/pdf" {
				n := i + 1
				pageNumber = &n
			}
			result.Facts = append(result.Facts, resume.Fact{ID: security.UUID(), Category: "other", Text: chunk, Evidence: resume.Evidence{Source: "file", Page: pageNumber, Excerpt: chunk}})
		}
	}
	if len(result.Facts) == 0 {
		return fail("resume_ocr_required")
	}
	return result, nil
}
