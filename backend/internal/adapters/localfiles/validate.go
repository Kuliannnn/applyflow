package localfiles

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"applyflow/backend/internal/files"
)

const docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

type Validator struct{ PDFInfo string }

func NewValidator(binary string) (Validator, error) {
	if binary == "" {
		binary = "pdfinfo"
	}
	p, err := exec.LookPath(binary)
	return Validator{PDFInfo: p}, err
}
func (v Validator) Validate(ctx context.Context, data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		// stdin prevents caller-controlled command options and temporary public paths.
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, v.PDFInfo, "-")
		cmd.Stdin = bytes.NewReader(data)
		cmd.Env = []string{"LC_ALL=C", "LANG=C"}
		out := &boundedOutput{}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			// Poppler exit 1 means the input could not be opened/read. Other
			// failures (missing executable, timeout, signal, output failure)
			// describe the validator, not a user's unsupported file format.
			if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 1 {
				return "", files.ValidationError("invalid_pdf")
			}
			if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 3 {
				return "", files.ValidationError("pdf_restricted")
			}
			return "", files.ErrValidatorUnavailable
		}
		pages := 0
		encrypted := true
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(line, "Pages:") {
				pages, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			}
			if strings.HasPrefix(line, "Encrypted:") {
				encrypted = !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, "Encrypted:")), "no")
			}
		}
		if pages < 1 {
			return "", files.ValidationError("invalid_pdf")
		}
		if encrypted {
			return "", files.ValidationError("pdf_restricted")
		}
		if pages > 20 {
			return "", files.ValidationError("pdf_page_limit")
		}
		return "application/pdf", nil
	}
	if err := validateDOCX(data); err != nil {
		return "", files.ErrUnsupported
	}
	return docxMIME, nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, errors.New("validator output limit")
	}
	return b.Buffer.Write(p)
}
func validateDOCX(data []byte) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(z.File) > 512 {
		return files.ErrUnsupported
	}
	seen := map[string]bool{}
	var total uint64
	hasDocument, hasType := false, false
	for _, f := range z.File {
		name := f.Name
		if seen[name] || strings.Contains(name, "\\") || path.IsAbs(name) || strings.HasPrefix(path.Clean(name), "../") || f.Mode()&os.ModeSymlink != 0 {
			return files.ErrUnsupported
		}
		seen[name] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(name), "vbaproject") || strings.HasPrefix(name, "word/embeddings/") {
			return files.ErrUnsupported
		}
		if f.UncompressedSize64 > 8*1024*1024 {
			return files.ErrUnsupported
		}
		total += f.UncompressedSize64
		if total > 32*1024*1024 {
			return files.ErrUnsupported
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(r, 8*1024*1024+1))
		_ = r.Close()
		if err != nil || len(b) > 8*1024*1024 {
			return files.ErrUnsupported
		}
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(b))
		depth, roots := 0, 0
		for {
			tok, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			switch t := tok.(type) {
			case xml.Directive:
				return files.ErrUnsupported
			case xml.StartElement:
				depth++
				if depth == 1 {
					roots++
				}
				if depth > 64 {
					return files.ErrUnsupported
				}
				if name == "word/document.xml" && depth == 1 && t.Name.Local == "document" && t.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
					hasDocument = true
				}
				attrs := map[string]string{}
				for _, a := range t.Attr {
					attrs[a.Name.Local] = a.Value
				}
				if name == "[Content_Types].xml" && t.Name.Local == "Override" && attrs["PartName"] == "/word/document.xml" && attrs["ContentType"] == "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml" {
					hasType = true
				}
				if strings.HasSuffix(name, ".rels") && t.Name.Local == "Relationship" && attrs["TargetMode"] == "External" && !strings.HasSuffix(attrs["Type"], "/hyperlink") {
					return files.ErrUnsupported
				}
			case xml.EndElement:
				depth--
			case xml.CharData:
				if depth == 0 && strings.TrimSpace(string(t)) != "" {
					return files.ErrUnsupported
				}
			}
		}
		if roots != 1 || depth != 0 {
			return files.ErrUnsupported
		}
	}
	if !hasDocument || !hasType {
		return files.ErrUnsupported
	}
	return nil
}
