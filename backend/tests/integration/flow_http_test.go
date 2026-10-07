package integration

import (
	"applyflow/backend/internal/adapters/localfiles"
	"applyflow/backend/internal/adapters/postgres"
	"applyflow/backend/internal/adapters/provider"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
	"applyflow/backend/internal/testfixture"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func bodyJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type flowFixture struct {
	blobs                    *localfiles.Storage
	a, b                     *browser
	db                       *sql.DB
	ctx                      context.Context
	store                    postgres.FlowStore
	workspace, revision, job string
	version                  int64
	source                   intake.Accepted
}

func newFlow(t *testing.T) flowFixture {
	t.Helper()
	h, db, ctx, blobs := newHTTPWithStorage(t)
	a, b := newBrowser(h), newBrowser(h)
	a.register(t, "flow@example.com")
	b.register(t, "other-flow@example.com")
	w := a.upload(t, "base.docx", testfixture.DOCX(nil))
	requireStatus(t, w, 201)
	f := decodeBody[files.File](t, w)
	w = a.request("POST", "/api/resumes", importBody(f.ID), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 201)
	r := decodeBody[struct {
		Resume resume.Resume `json:"resume"`
	}](t, w).Resume
	w = a.request("POST", "/api/resumes/"+r.ID+"/revisions", factBody(1, "Built APIs"), nil)
	requireStatus(t, w, 201)
	rev := decodeBody[resume.Confirmed](t, w).Revision.ID
	w = a.request("POST", "/api/workspaces", `{}`, map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 201)
	workspace := decodeBody[struct {
		ID string `json:"id"`
	}](t, w).ID
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+workspace, fmt.Sprintf(`{"expected_version":1,"resume_revision_id":%q}`, rev), nil), 200)
	sourceBody := bodyJSON(t, intake.Create{ExpectedVersion: 2, Source: intake.TextSource{Kind: "text", Text: strings.Repeat("Go engineer. ", 3000)}})
	key := security.UUID()
	w = a.request("POST", "/api/workspaces/"+workspace+"/sources", sourceBody, map[string]string{"Idempotency-Key": key})
	requireStatus(t, w, 202)
	source := decodeBody[intake.Accepted](t, w)
	replay := a.request("POST", "/api/workspaces/"+workspace+"/sources", sourceBody, map[string]string{"Idempotency-Key": key})
	requireStatus(t, replay, 202)
	if replay.Body.String() != w.Body.String() {
		t.Fatal("large source replay differs")
	}
	requireStatus(t, b.request("GET", "/api/workspaces/"+workspace+"/sources/"+source.Source.ID, "", nil), 404)
	store := postgres.FlowStore{DB: db}
	worker := task.Worker{Store: store, Executor: generation.Executor{Store: store, Provider: provider.Mock{}}}
	worked, err := worker.Tick(ctx)
	if err != nil || !worked {
		t.Fatalf("extract tick: %v", err)
	}
	w = a.request("GET", "/api/tasks/"+source.TaskID, "", nil)
	requireStatus(t, w, 200)
	snapshot := decodeBody[task.Snapshot](t, w)
	if snapshot.Status != "completed" {
		t.Fatal("text extraction did not complete")
	}
	if strings.Contains(w.Body.String(), "input_snapshot") || strings.Contains(w.Body.String(), "fencing_token") {
		t.Fatal("unsafe task response")
	}
	in := intake.Confirm{ExpectedVersion: 3, SourceID: source.Source.ID, SourceVersion: 1, Company: "Northstar", RoleTitle: "Backend Engineer", Description: "Build Go services."}
	w = a.request("POST", "/api/workspaces/"+workspace+"/job-revisions", bodyJSON(t, in), nil)
	requireStatus(t, w, 201)
	job := decodeBody[intake.Confirmed](t, w)
	return flowFixture{blobs: blobs, a: a, b: b, db: db, ctx: ctx, store: store, workspace: workspace, revision: rev, job: job.Revision.ID, version: job.WorkspaceVersion, source: source}
}
func (f flowFixture) generateBody(t *testing.T) string {
	return bodyJSON(t, studio.Generate{ExpectedVersion: f.version, JobRevisionID: f.job, ResumeRevisionID: f.revision, ProfileVersion: 1, ExecutionMode: "mock", Locale: "en"})
}
func (f *flowFixture) generate(t *testing.T) studio.GenerationAccepted {
	t.Helper()
	w := f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", f.generateBody(t), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	out := decodeBody[studio.GenerationAccepted](t, w)
	f.version = out.WorkspaceVersion
	return out
}
func (f flowFixture) drain(t *testing.T) {
	t.Helper()
	worker := task.Worker{Store: f.store, Executor: f.executor()}
	for i := 0; i < 10; i++ {
		worked, err := worker.Tick(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("worker did not drain")
}
func (f flowFixture) generation(t *testing.T, run string) studio.Generation {
	t.Helper()
	w := f.a.request("GET", "/api/workspaces/"+f.workspace+"/generations/"+run, "", nil)
	requireStatus(t, w, 200)
	return decodeBody[studio.Generation](t, w)
}
func resultIDs(t *testing.T, s task.Snapshot) (string, string) {
	t.Helper()
	var v struct {
		DocumentID string `json:"document_id"`
		RevisionID string `json:"revision_id"`
	}
	if err := json.Unmarshal(s.Result, &v); err != nil {
		t.Fatal(err)
	}
	return v.DocumentID, v.RevisionID
}
func TestHTTPTextToTwoDocuments(t *testing.T) {
	f := newFlow(t)
	key := security.UUID()
	body := f.generateBody(t)
	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", body, map[string]string{"Idempotency-Key": key})
		}(i)
	}
	wg.Wait()
	for _, w := range results {
		requireStatus(t, w, 202)
	}
	if results[0].Body.String() != results[1].Body.String() {
		t.Fatal("generation replay differs")
	}
	accepted := decodeBody[studio.GenerationAccepted](t, results[0])
	f.version = accepted.WorkspaceVersion
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM generation_runs`) != 1 || scalar(t, f.ctx, f.db, `SELECT count(*) FROM task_outbox`) != 3 {
		t.Fatal("acceptance not atomic/idempotent")
	}
	requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", f.generateBody(t), map[string]string{"Idempotency-Key": security.UUID()}), 409)
	requireStatus(t, f.b.request("GET", "/api/tasks/"+accepted.ResumeTaskID, "", nil), 404)
	requireStatus(t, f.b.request("POST", "/api/tasks/"+accepted.ResumeTaskID+"/cancel", "", nil), 404)
	requireStatus(t, f.b.request("GET", "/api/workspaces/"+f.workspace+"/generations/"+accepted.RunID, "", nil), 404)
	f.drain(t)
	g := f.generation(t, accepted.RunID)
	if g.ResumeTask.Status != "completed" || g.CoverLetterTask.Status != "completed" {
		t.Fatal("two independent drafts not completed")
	}
	docID, revID := resultIDs(t, g.ResumeTask)
	w := f.a.request("GET", "/api/documents/"+docID, "", nil)
	requireStatus(t, w, 200)
	doc := decodeBody[document.Document](t, w)
	if doc.CurrentRevisionID == nil || *doc.CurrentRevisionID != revID || doc.Version != 2 {
		t.Fatal("first draft not initialized")
	}
	requireStatus(t, f.b.request("GET", "/api/documents/"+docID+"/revisions/"+revID, "", nil), 404)
	w = f.a.request("GET", "/api/documents/"+docID+"/revisions/"+revID, "", nil)
	requireStatus(t, w, 200)
	rev := decodeBody[document.Revision](t, w)
	if rev.Content.Title == nil || !strings.Contains(*rev.Content.Title, "Mock") || rev.Content.Sections[0].Items[0].Text != "Built APIs" {
		t.Fatal("mock hid its identity or invented facts")
	}
	// An in-flight newer result must remain a candidate after a human edit.
	next := f.generate(t)
	rev.Content.Sections[0].Items[0].Text = "My reviewed wording"
	save := document.Save{ExpectedVersion: 2, BaseRevisionID: revID, Content: rev.Content}
	w = f.a.request("POST", "/api/documents/"+docID+"/revisions", bodyJSON(t, save), nil)
	requireStatus(t, w, 201)
	saved := decodeBody[document.Saved](t, w)
	requireStatus(t, f.a.request("POST", "/api/documents/"+docID+"/revisions", bodyJSON(t, save), nil), 409)
	f.drain(t)
	nextG := f.generation(t, next.RunID)
	_, candidate := resultIDs(t, nextG.ResumeTask)
	w = f.a.request("GET", "/api/documents/"+docID, "", nil)
	requireStatus(t, w, 200)
	doc = decodeBody[document.Document](t, w)
	if doc.CurrentRevisionID == nil || *doc.CurrentRevisionID != saved.Revision.ID {
		t.Fatal("late result overwrote manual head")
	}
	requireStatus(t, f.a.request("POST", "/api/documents/"+docID+"/apply", bodyJSON(t, document.Apply{ExpectedVersion: doc.Version, RevisionID: revID}), nil), 409)
	requireStatus(t, f.a.request("POST", "/api/documents/"+docID+"/apply", bodyJSON(t, document.Apply{ExpectedVersion: doc.Version, RevisionID: candidate}), nil), 200)
	w = f.a.request("GET", "/api/documents/"+docID+"/revisions?limit=1", "", nil)
	requireStatus(t, w, 200)
	page := decodeBody[struct {
		Next string `json:"next_cursor"`
	}](t, w)
	requireStatus(t, f.a.request("GET", "/api/documents/"+docID+"/revisions?cursor="+page.Next, "", nil), 200)
	letterID, _ := resultIDs(t, nextG.CoverLetterTask)
	requireStatus(t, f.a.request("GET", "/api/documents/"+letterID+"/revisions?cursor="+page.Next, "", nil), 400)
	replay := f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", body, map[string]string{"Idempotency-Key": key})
	requireStatus(t, replay, 202)
	if replay.Body.String() != results[0].Body.String() {
		t.Fatal("run replay changed after later edits")
	}
}
func TestHTTPPartialCancellationAndRetry(t *testing.T) {
	f := newFlow(t)
	run := f.generate(t)
	requireStatus(t, f.a.request("POST", "/api/tasks/"+run.CoverLetterTaskID+"/cancel", "", nil), 200)
	f.drain(t)
	g := f.generation(t, run.RunID)
	if g.ResumeTask.Status != "completed" || g.CoverLetterTask.Status != "cancelled" {
		t.Fatal("cancel erased successful sibling")
	}
	before := string(g.ResumeTask.Result)
	retry := bodyJSON(t, studio.Retry{ExpectedVersion: f.version, Kind: "cover_letter"})
	key := security.UUID()
	path := "/api/workspaces/" + f.workspace + "/generations/" + run.RunID + "/retry"
	w := f.a.request("POST", path, retry, map[string]string{"Idempotency-Key": key})
	requireStatus(t, w, 202)
	newTask := decodeBody[studio.TaskAccepted](t, w).TaskID
	f.drain(t)
	g = f.generation(t, run.RunID)
	if g.CoverLetterTask.ID != newTask || g.CoverLetterTask.Status != "completed" || string(g.ResumeTask.Result) != before {
		t.Fatal("retry regenerated sibling or lost original snapshot")
	}
	replay := f.a.request("POST", path, retry, map[string]string{"Idempotency-Key": key})
	requireStatus(t, replay, 202)
	if replay.Body.String() != w.Body.String() {
		t.Fatal("retry replay changed")
	}
	requireStatus(t, f.a.request("POST", "/api/tasks/"+newTask+"/cancel", "", nil), 200)
	snapshot, err := f.store.Task(f.ctx, g.CoverLetterTask.ID, g.CoverLetterTask.ID)
	if err == nil || snapshot.ID != "" {
		t.Fatal("task read ignored owner")
	}
}
func TestHTTPFlowValidationAndRollback(t *testing.T) {
	f := newFlow(t)
	base := f.generateBody(t)
	for _, bad := range []string{strings.Replace(base, `"mock"`, `"personal"`, 1), strings.Replace(base, `"expected_profile_version":1`, `"expected_profile_version":2`, 1)} {
		status := 400
		if strings.Contains(bad, `"expected_profile_version":2`) {
			status = 409
		}
		requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", bad, map[string]string{"Idempotency-Key": security.UUID()}), status)
	}
	// Force failure after run/resume-task creation, proving the accepting transaction rolls back all rows.
	exec(t, f.ctx, f.db, `CREATE FUNCTION reject_letter() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='write_cover_letter' THEN RAISE EXCEPTION 'test failure'; END IF; RETURN NEW; END; $$`)
	exec(t, f.ctx, f.db, `CREATE TRIGGER reject_letter BEFORE INSERT ON job_tasks FOR EACH ROW EXECUTE FUNCTION reject_letter()`)
	requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/generations", base, map[string]string{"Idempotency-Key": security.UUID()}), 503)
	if scalar(t, f.ctx, f.db, `SELECT count(*) FROM generation_runs`) != 0 || scalar(t, f.ctx, f.db, `SELECT count(*) FROM documents`) != 0 || scalar(t, f.ctx, f.db, `SELECT count(*) FROM job_tasks`) != 1 {
		t.Fatal("failed accept leaked partial records")
	}
	exec(t, f.ctx, f.db, `DROP TRIGGER reject_letter ON job_tasks`)
	run := f.generate(t)
	f.drain(t)
	g := f.generation(t, run.RunID)
	id, revID := resultIDs(t, g.ResumeTask)
	w := f.a.request("GET", "/api/documents/"+id+"/revisions/"+revID, "", nil)
	requireStatus(t, w, 200)
	rev := decodeBody[document.Revision](t, w)
	rev.Content.Sections[0].Items[0].FactIDs = []string{security.UUID()}
	requireStatus(t, f.a.request("POST", "/api/documents/"+id+"/revisions", bodyJSON(t, document.Save{ExpectedVersion: 2, BaseRevisionID: revID, Content: rev.Content}), nil), 400)
	for _, content := range []string{`{"kind":"resume","title":"CV","sections":[],"paragraphs":null}`, `{"kind":"resume","Title":"CV","sections":[]}`, `{"kind":"cover_letter","salutation":"Hello","paragraphs":[{"text":"Hello","fact_ids":null}],"closing":"Bye"}`} {
		requireStatus(t, f.a.request("POST", "/api/documents/"+id+"/revisions", fmt.Sprintf(`{"expected_version":2,"base_revision_id":%q,"content":%s}`, revID, content), nil), 400)
	}
	// Replacing text invalidates current JD selection; stale confirmation cannot attach it again.
	w = f.a.request("POST", "/api/workspaces/"+f.workspace+"/sources", bodyJSON(t, intake.Create{ExpectedVersion: f.version, Source: intake.TextSource{Kind: "text", Text: "Different role"}}), map[string]string{"Idempotency-Key": security.UUID()})
	requireStatus(t, w, 202)
	source := decodeBody[intake.Accepted](t, w)
	stale := intake.Confirm{ExpectedVersion: source.WorkspaceVersion, SourceID: f.source.Source.ID, SourceVersion: 1, Company: "Old", RoleTitle: "Old", Description: "Old"}
	requireStatus(t, f.a.request("POST", "/api/workspaces/"+f.workspace+"/job-revisions", bodyJSON(t, stale), nil), 409)
}

func (f flowFixture) executor() generation.Executor {
	return generation.Executor{Store: f.store, Provider: provider.Mock{}}
}

func TestGenerationNeedsJDNotSeparateMetadata(t *testing.T) {
	f := newFlow(t)
	description := "Northstar is hiring a Backend Engineer to build Go services."
	in := intake.Confirm{ExpectedVersion: f.version, SourceID: f.source.Source.ID, SourceVersion: 1, Description: description}
	response := f.a.request("POST", "/api/workspaces/"+f.workspace+"/job-revisions", bodyJSON(t, in), nil)
	requireStatus(t, response, 201)
	confirmed := decodeBody[intake.Confirmed](t, response)
	if confirmed.Revision.Company != "" || confirmed.Revision.RoleTitle != "" || confirmed.Revision.Description != description {
		t.Fatal("unknown metadata or full JD was changed")
	}
	f.job, f.version = confirmed.Revision.ID, confirmed.WorkspaceVersion
	accepted := f.generate(t)
	f.drain(t)
	result := f.generation(t, accepted.RunID)
	if result.ResumeTask.Status != "completed" || result.CoverLetterTask.Status != "completed" {
		t.Fatal("empty metadata blocked document generation")
	}
}

func TestRegenerationPromotesUnchangedDocument(t *testing.T) {
	f := newFlow(t)
	first := f.generate(t)
	f.drain(t)
	old := f.generation(t, first.RunID)
	docID, oldRevision := resultIDs(t, old.ResumeTask)
	second := f.generate(t)
	f.drain(t)
	next := f.generation(t, second.RunID)
	_, newRevision := resultIDs(t, next.ResumeTask)
	response := f.a.request("GET", "/api/documents/"+docID, "", nil)
	requireStatus(t, response, 200)
	head := decodeBody[document.Document](t, response)
	if oldRevision == newRevision || head.CurrentRevisionID == nil || *head.CurrentRevisionID != newRevision {
		t.Fatal("regeneration still displays old document")
	}
}
