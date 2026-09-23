package integration

import (
	"applyflow/backend/internal/adapters/render"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/task"
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func exportInput(t *testing.T, revision, format string) string {
	return bodyJSON(t, export.Create{RevisionID: revision, Format: format, TemplateVersion: "1"})
}
func (f flowFixture) exportExecutor(t *testing.T) export.Executor {
	t.Helper()
	r, err := render.New(f.ctx, os.Getenv("EXPORT_PYTHON"))
	if err != nil {
		t.Fatal(err)
	}
	return export.Executor{Store: f.store, Renderer: r, Blobs: f.blobs}
}
func (f flowFixture) exportTick(t *testing.T) {
	t.Helper()
	worked, err := (task.Worker{Store: f.store, Executor: f.exportExecutor(t)}).Tick(f.ctx)
	if err != nil || !worked {
		t.Fatalf("export tick: worked=%v err=%v", worked, err)
	}
}
func TestHTTPExportSnapshotsAndDeduplication(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	f.drain(t)
	g := f.generation(t, run.RunID)
	docID, revID := resultIDs(t, g.ResumeTask)
	w := f.a.request("GET", "/api/documents/"+docID+"/revisions/"+revID, "", nil)
	requireStatus(t, w, 200)
	rev := decodeBody[document.Revision](t, w)
	path := "/api/documents/" + docID + "/exports"
	body := exportInput(t, revID, "pdf")
	key := security.UUID()
	headers := map[string]string{"Idempotency-Key": key}
	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = f.a.request("POST", path, body, headers) }(i)
	}
	wg.Wait()
	for _, r := range results {
		requireStatus(t, r, 202)
	}
	if results[0].Body.String() != results[1].Body.String() {
		t.Fatal("export replay differs")
	}
	original := results[0].Body.String()
	x := decodeBody[export.Export](t, results[0])
	w = f.a.request("POST", path, body, map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	if decodeBody[export.Export](t, w).TaskID != x.TaskID {
		t.Fatal("active export duplicated")
	}
	requireStatus(t, f.b.request("POST", path, body, map[string]string{"Idempotency-Key": security.UUID()}), 404)
	requireStatus(t, f.b.request("GET", path+"/"+x.ID, "", nil), 404)
	requireStatus(t, f.a.request("POST", path, exportInput(t, revID, "docx"), headers), 409)
	docxResponse := f.a.request("POST", path, exportInput(t, revID, "docx"), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, docxResponse, 202)
	dx := decodeBody[export.Export](t, docxResponse)
	// Update head after acceptance, before rendering either artifact.
	edited := rev.Content
	edited.Sections[0].Items[0].Text = "Edited after export was accepted"
	requireStatus(t, f.a.request("POST", "/api/documents/"+docID+"/revisions", bodyJSON(t, document.Save{ExpectedVersion: 2, BaseRevisionID: revID, Content: edited}), nil), 201)
	f.exportTick(t)
	f.exportTick(t)
	for _, artifact := range []export.Export{x, dx} {
		w = f.a.request("GET", path+"/"+artifact.ID, "", nil)
		requireStatus(t, w, 200)
		ready := decodeBody[export.Export](t, w)
		if ready.FileID == nil || ready.RevisionID != revID {
			t.Fatal("artifact not pinned to accepted revision")
		}
		requireStatus(t, f.b.request("GET", "/api/files/"+*ready.FileID+"/download", "", nil), 404)
		download := f.a.request("GET", "/api/files/"+*ready.FileID+"/download", "", nil)
		requireStatus(t, download, 200)
		if !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment;") || download.Header().Get("Content-Type") != "application/octet-stream" {
			t.Fatal("unsafe download headers")
		}
		if artifact.Format == "pdf" {
			// Render immutable content again; invariant PDF bytes prove the stored file used the old revision.
			old, err := f.store.DocumentRevision(f.ctx, rowID(t, f.ctx, f.db, `SELECT owner_id FROM documents WHERE id=$1`, docID), docID, revID)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := f.exportExecutor(t).Renderer.Render(f.ctx, "pdf", old.Content)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expected, download.Body.Bytes()) {
				t.Fatal("PDF content changed with head")
			}
		} else {
			z, err := zip.NewReader(bytes.NewReader(download.Body.Bytes()), int64(download.Body.Len()))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, part := range z.File {
				if part.Name == "word/document.xml" {
					p, err := part.Open()
					if err != nil {
						t.Fatal(err)
					}
					raw, err := io.ReadAll(p)
					p.Close()
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(raw, []byte("Built APIs")) || bytes.Contains(raw, []byte("Edited after export")) {
						t.Fatal("DOCX used mutable head")
					}
					found = true
				}
			}
			if !found {
				t.Fatal("DOCX lacks editable body")
			}
		}
	}
	w = f.a.request("POST", path, body, headers)
	requireStatus(t, w, 202)
	if w.Body.String() != original {
		t.Fatal("original replay must remain stable")
	}
	w = f.a.request("POST", path, body, map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 200)
	if decodeBody[export.Export](t, w).ID != x.ID {
		t.Fatal("ready artifact not reused")
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM document_exports`) != 2 || scalar(t, f.ctx, f.db, `SELECT count(*) FROM job_tasks WHERE kind='export_document'`) != 2 {
		t.Fatal("duplicate artifact/task")
	}
}
func TestHTTPExportRetryAndRenderingFailure(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	f.drain(t)
	g := f.generation(t, run.RunID)
	doc, revision := resultIDs(t, g.CoverLetterTask)
	path := "/api/documents/" + doc + "/exports"
	body := exportInput(t, revision, "pdf")
	key := security.UUID()
	w := f.a.request("POST", path, body, map[string]string{"Idempotency-Key": key})
	requireStatus(t, w, 202)
	x := decodeBody[export.Export](t, w)
	requireStatus(t, f.a.request("POST", "/api/tasks/"+x.TaskID+"/cancel", "", nil), 200)
	w = f.a.request("POST", path, body, map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	replacement := decodeBody[export.Export](t, w)
	if replacement.ID != x.ID || replacement.TaskID == x.TaskID {
		t.Fatal("retry must preserve export identity and replace task")
	}
	f.exportTick(t)
	w = f.a.request("GET", "/api/documents/"+doc+"/revisions/"+revision, "", nil)
	requireStatus(t, w, 200)
	r := decodeBody[document.Revision](t, w)
	r.Content.Paragraphs[0].Text = "Unsupported character 😀"
	w = f.a.request("POST", "/api/documents/"+doc+"/revisions", bodyJSON(t, document.Save{ExpectedVersion: 2, BaseRevisionID: revision, Content: r.Content}), nil)
	requireStatus(t, w, 201)
	saved := decodeBody[document.Saved](t, w)
	w = f.a.request("POST", path, exportInput(t, saved.Revision.ID, "pdf"), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	bad := decodeBody[export.Export](t, w)
	worked, err := (task.Worker{Store: f.store, Executor: f.exportExecutor(t)}).Tick(f.ctx)
	if !worked || err == nil {
		t.Fatal("unsupported character accepted")
	}
	w = f.a.request("GET", "/api/tasks/"+bad.TaskID, "", nil)
	requireStatus(t, w, 200)
	state := decodeBody[task.Snapshot](t, w)
	if state.Status != "failed" || state.ErrorCode == nil || *state.ErrorCode != "export_unsupported_character" {
		t.Fatal("missing safe actionable renderer error")
	}
	w = f.a.request("GET", path+"/"+bad.ID, "", nil)
	requireStatus(t, w, 200)
	if decodeBody[export.Export](t, w).FileID != nil {
		t.Fatal("failed artifact exposed a file")
	}
}
func TestExportFencesAndAtomicFileCommit(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	f.drain(t)
	g := f.generation(t, run.RunID)
	doc, revision := resultIDs(t, g.ResumeTask)
	path := "/api/documents/" + doc + "/exports"
	w := f.a.request("POST", path, exportInput(t, revision, "pdf"), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	x := decodeBody[export.Export](t, w)
	c, err := f.store.Claim(f.ctx)
	if err != nil || c == nil {
		t.Fatal("claim", err)
	}
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID)
	if err = f.exportExecutor(t).Execute(f.ctx, *c); !errors.Is(err, task.ErrLeaseLost) {
		t.Fatal("expired exporter committed", err)
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM files WHERE purpose='export'`) != 0 {
		t.Fatal("expired file metadata leaked")
	}
	requireStatus(t, f.a.request("POST", "/api/tasks/"+x.TaskID+"/cancel", "", nil), 200)
	w = f.a.request("POST", path, exportInput(t, revision, "pdf"), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	exec(t, f.ctx, f.db, `CREATE FUNCTION reject_export_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.purpose='export' THEN RAISE EXCEPTION 'synthetic file failure'; END IF; RETURN NEW; END; $$`)
	exec(t, f.ctx, f.db, `CREATE TRIGGER reject_export_file BEFORE INSERT ON files FOR EACH ROW EXECUTE FUNCTION reject_export_file()`)
	worked, err := (task.Worker{Store: f.store, Executor: f.exportExecutor(t)}).Tick(f.ctx)
	if !worked || err == nil {
		t.Fatal("expected file commit failure")
	}
	w = f.a.request("GET", path+"/"+x.ID, "", nil)
	requireStatus(t, w, 200)
	if decodeBody[export.Export](t, w).FileID != nil {
		t.Fatal("failed metadata commit linked a file")
	}
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM files WHERE purpose='export'`) != 0 {
		t.Fatal("failed file commit was not rolled back")
	}
	var result map[string]any
	w = f.a.request("GET", "/api/tasks/"+decodeBody[export.Export](t, w).TaskID, "", nil)
	requireStatus(t, w, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "failed" || result["result"] != nil {
		t.Fatal("task falsely completed")
	}
}
