// Package render runs the embedded, fixed template in an isolated Python interpreter.
package render

import (
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/files"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

//go:embed render.py
var script string

//go:embed render_v1.py
var legacyScript string

type Renderer struct{ Python string }

func New(ctx context.Context, binary string) (Renderer, error) {
	if binary == "" {
		binary = "python3"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return Renderer{}, errors.New("EXPORT_PYTHON unavailable")
	}
	r := Renderer{Python: path}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(check, path, "-I", "-c", "import reportlab, docx; assert reportlab.Version == '4.4.9' and docx.__version__ == '1.2.0'")
	if err = cmd.Run(); err != nil {
		return r, errors.New("install the pinned export renderer dependencies")
	}
	return r, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("renderer output limit")
	}
	return b.Buffer.Write(p)
}
func (r Renderer) Render(ctx context.Context, format, template string, content document.Content) ([]byte, error) {
	if format != "pdf" && format != "docx" {
		return nil, export.Error("export_format_invalid")
	}
	selected := script
	if template == "1" {
		selected = legacyScript
	} else if template != "2" {
		return nil, export.Error("export_template_invalid")
	}
	raw, err := json.Marshal(map[string]any{"format": format, "content": content})
	if err != nil {
		return nil, err
	}
	if len(raw) > 262144 {
		return nil, export.Error("export_size_limit")
	}
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, r.Python, "-I", "-c", selected)
	cmd.Env = []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	cmd.Stdin = bytes.NewReader(raw)
	out, stderr := &boundedBuffer{limit: files.MaxBytes}, &boundedBuffer{limit: 1024}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		if bounded.Err() != nil {
			return nil, export.Error("export_render_timeout")
		}
		switch strings.TrimSpace(stderr.String()) {
		case "export_unsupported_character":
			return nil, export.Error("export_unsupported_character")
		case "export_page_limit":
			return nil, export.Error("export_page_limit")
		case "export_size_limit":
			return nil, export.Error("export_size_limit")
		}
		return nil, export.Error("export_render_failed")
	}
	if (format == "pdf" && !bytes.HasPrefix(out.Bytes(), []byte("%PDF-"))) || (format == "docx" && !bytes.HasPrefix(out.Bytes(), []byte("PK"))) {
		return nil, export.Error("export_render_failed")
	}
	return out.Bytes(), nil
}
