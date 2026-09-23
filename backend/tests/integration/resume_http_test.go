package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/testfixture"
)

func (b *browser) upload(t *testing.T, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("purpose", "base_resume"); err != nil {
		t.Fatal(err)
	}
	p, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.request("POST", "/api/files", body.String(), map[string]string{"Content-Type": w.FormDataContentType()})
}
func decodeBody[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func importBody(file string) string {
	return fmt.Sprintf(`{"file_id":%q,"name":"Base","import_mode":"manual"}`, file)
}
func factBody(version int, text string) string {
	return fmt.Sprintf(`{"expected_version":%d,"facts":[{"id":"10000000-0000-4000-8000-000000000001","category":"experience","text":%q,"evidence":{"source":"user","page":null,"excerpt":""}}]}`, version, text)
}
func TestHTTPResumeLifecycle(t *testing.T) {
	h, db, ctx := newHTTP(t)
	a, b := newBrowser(h), newBrowser(h)
	a.register(t, "resume-a@example.com")
	b.register(t, "resume-b@example.com")
	raw := testfixture.DOCX(nil)
	w := a.upload(t, "简历.docx", raw)
	requireStatus(t, w, 201)
	f := decodeBody[files.File](t, w)
	if strings.Contains(w.Body.String(), "storage_key") {
		t.Fatal("storage key exposed")
	}
	requireStatus(t, b.request("GET", "/api/files/"+f.ID+"/download", "", nil), 404)
	w = a.request("GET", "/api/files/"+f.ID+"/download", "", nil)
	requireStatus(t, w, 200)
	if !bytes.Equal(w.Body.Bytes(), raw) || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unsafe download or changed bytes")
	}
	key := security.UUID()
	headers := map[string]string{"Idempotency-Key": key}
	requireStatus(t, b.request("POST", "/api/resumes", importBody(f.ID), headers), 404)
	w = a.request("POST", "/api/resumes", importBody(f.ID), headers)
	requireStatus(t, w, 201)
	imported := decodeBody[struct {
		Resume resume.Resume `json:"resume"`
		TaskID *string       `json:"task_id"`
	}](t, w)
	r := imported.Resume
	original := w.Body.String()
	if imported.TaskID != nil || r.CurrentRevisionID != nil {
		t.Fatal("manual import created phantom parsing result")
	}
	requireStatus(t, a.request("PATCH", "/api/resumes/"+r.ID, `{"expected_version":1,"is_default":true}`, nil), 409)
	requireStatus(t, b.request("GET", "/api/resumes/"+r.ID, "", nil), 404)
	w = a.request("POST", "/api/resumes/"+r.ID+"/revisions", factBody(1, "Built APIs"), nil)
	requireStatus(t, w, 201)
	first := decodeBody[resume.Confirmed](t, w)
	if first.ResumeVersion != 2 || first.Revision.ParentRevisionID != nil {
		t.Fatal("invalid first revision")
	}
	requireStatus(t, b.request("GET", "/api/resumes/"+r.ID+"/revisions/"+first.Revision.ID, "", nil), 404)
	requireStatus(t, a.request("POST", "/api/resumes/"+r.ID+"/revisions", factBody(1, "Stale overwrite"), nil), 409)
	requireStatus(t, a.request("PATCH", "/api/resumes/"+r.ID, `{"expected_version":2,"is_default":true}`, nil), 200)
	w = a.request("POST", "/api/workspaces", `{}`, map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 201)
	workspace := decodeBody[struct {
		ID string `json:"id"`
	}](t, w)
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+workspace.ID, fmt.Sprintf(`{"expected_version":1,"resume_revision_id":%q}`, first.Revision.ID), nil), 200)
	w = a.request("POST", "/api/resumes/"+r.ID+"/revisions", factBody(3, "Improved APIs"), nil)
	requireStatus(t, w, 201)
	second := decodeBody[resume.Confirmed](t, w)
	if second.Revision.ParentRevisionID == nil || *second.Revision.ParentRevisionID != first.Revision.ID {
		t.Fatal("broken revision chain")
	}
	w = a.request("GET", "/api/workspaces/"+workspace.ID, "", nil)
	requireStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), first.Revision.ID) {
		t.Fatal("workspace source silently advanced")
	}
	w = a.request("GET", "/api/resumes/"+r.ID+"/revisions/"+first.Revision.ID, "", nil)
	requireStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), "Built APIs") {
		t.Fatal("old facts overwritten")
	}
	w = a.request("POST", "/api/resumes", importBody(f.ID), headers)
	requireStatus(t, w, 201)
	if w.Body.String() != original {
		t.Fatal("replay response changed")
	}
	requireStatus(t, a.request("POST", "/api/resumes", strings.Replace(importBody(f.ID), "Base", "Changed", 1), headers), 409)
	if scalar(t, ctx, db, `SELECT count(*) FROM job_tasks`) != 0 {
		t.Fatal("manual import queued a task")
	}
	// A second confirmed resume can replace the default atomically.
	w = a.request("POST", "/api/resumes", importBody(f.ID), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 201)
	r2 := decodeBody[struct {
		Resume resume.Resume `json:"resume"`
	}](t, w).Resume
	requireStatus(t, a.request("POST", "/api/resumes/"+r2.ID+"/revisions", factBody(1, "Second CV"), nil), 201)
	requireStatus(t, a.request("PATCH", "/api/resumes/"+r2.ID, `{"expected_version":2,"is_default":true}`, nil), 200)
	if scalar(t, ctx, db, `SELECT count(*) FROM resumes WHERE is_default`) != 1 {
		t.Fatal("multiple defaults")
	}
	w = a.request("GET", "/api/resumes/"+r.ID, "", nil)
	requireStatus(t, w, 200)
	old := decodeBody[resume.Resume](t, w)
	if old.IsDefault || old.Version != 5 {
		t.Fatal("previous default/version not updated")
	}
	w = a.request("GET", "/api/resumes?limit=1", "", nil)
	requireStatus(t, w, 200)
	page := decodeBody[struct {
		Next string `json:"next_cursor"`
	}](t, w)
	requireStatus(t, a.request("GET", "/api/resumes?limit=1&cursor="+page.Next, "", nil), 200)
	requireStatus(t, b.request("GET", "/api/resumes?cursor="+page.Next, "", nil), 400)
	requireStatus(t, a.request("GET", "/api/workspaces?cursor="+page.Next, "", nil), 400)
	requireStatus(t, a.request("GET", "/api/resumes/"+r2.ID+"/revisions/"+first.Revision.ID, "", nil), 404)
}
func TestHTTPFileAndFactRejection(t *testing.T) {
	h, _, _ := newHTTP(t)
	a := newBrowser(h)
	requireStatus(t, a.upload(t, "cv.pdf", testfixture.PDF(1)), 401)
	a.register(t, "validation@example.com")
	for _, tc := range []struct {
		name   string
		data   []byte
		status int
	}{
		{"cv.pdf", []byte("fake PDF"), 415}, {"cv.pdf", testfixture.PDF(21), 415}, {"../cv.docx", testfixture.DOCX(nil), 400},
		{"cv.pdf", make([]byte, files.MaxBytes+1), 413}, {"cv.docx", testfixture.DOCX(map[string]string{"word/vbaProject.bin": "macro"}), 415},
	} {
		requireStatus(t, a.upload(t, tc.name, tc.data), tc.status)
	}
	w := a.upload(t, "cv.pdf", testfixture.PDF(1))
	requireStatus(t, w, 201)
	f := decodeBody[files.File](t, w)
	w = a.request("POST", "/api/resumes", importBody(f.ID), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 201)
	r := decodeBody[struct {
		Resume resume.Resume `json:"resume"`
	}](t, w).Resume
	valid := factBody(1, "Experience")
	invalid := []string{strings.Replace(valid, `"source"`, `"Source"`, 1), strings.Replace(valid, `"excerpt":""`, `"excerpt":null`, 1), strings.Replace(valid, `"page":null,`, "", 1), strings.Replace(valid, `"page":null`, `"page":21`, 1), strings.Replace(valid, `"Experience"`, `" "`, 1), strings.Replace(valid, `"Experience"`, `"\u0000"`, 1)}
	var duplicate map[string]any
	_ = json.Unmarshal([]byte(valid), &duplicate)
	facts := duplicate["facts"].([]any)
	duplicate["facts"] = append(facts, facts[0])
	raw, _ := json.Marshal(duplicate)
	invalid = append(invalid, string(raw))
	for _, body := range invalid {
		requireStatus(t, a.request("POST", "/api/resumes/"+r.ID+"/revisions", body, nil), 400)
	}
	// Confirmation deliberately accepts >128KiB, up to its own 256KiB bound.
	facts = make([]any, 40)
	for i := range facts {
		facts[i] = map[string]any{"id": security.UUID(), "category": "other", "text": strings.Repeat("x", 3500), "evidence": map[string]any{"source": "user", "page": nil, "excerpt": ""}}
	}
	raw, _ = json.Marshal(map[string]any{"expected_version": 1, "facts": facts})
	requireStatus(t, a.request("POST", "/api/resumes/"+r.ID+"/revisions", string(raw), nil), 201)
	requireStatus(t, a.request("POST", "/api/resumes/"+r.ID+"/revisions", strings.Repeat("x", 262145), nil), 413)
}
func TestHTTPResumeConcurrentWrites(t *testing.T) {
	h, db, ctx := newHTTP(t)
	a := newBrowser(h)
	a.register(t, "concurrent-resume@example.com")
	w := a.upload(t, "cv.docx", testfixture.DOCX(nil))
	requireStatus(t, w, 201)
	f := decodeBody[files.File](t, w)
	key := security.UUID()
	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = a.request("POST", "/api/resumes", importBody(f.ID), map[string]string{"Idempotency-Key": key})
		}(i)
	}
	wg.Wait()
	for _, w := range results {
		requireStatus(t, w, 201)
	}
	if results[0].Body.String() != results[1].Body.String() || scalar(t, ctx, db, `SELECT count(*) FROM resumes`) != 1 {
		t.Fatal("concurrent import duplicated")
	}
	r := decodeBody[struct {
		Resume resume.Resume `json:"resume"`
	}](t, results[0]).Resume
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = a.request("POST", "/api/resumes/"+r.ID+"/revisions", factBody(1, fmt.Sprintf("edit %d", i)), nil)
		}(i)
	}
	wg.Wait()
	if (results[0].Code != 201 || results[1].Code != 409) && (results[0].Code != 409 || results[1].Code != 201) {
		t.Fatalf("CAS outcomes %d %d", results[0].Code, results[1].Code)
	}
	if scalar(t, ctx, db, `SELECT count(*) FROM resume_revisions`) != 1 {
		t.Fatal("conflict left an orphan revision")
	}
}
