package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"applyflow/backend/internal/adapters/credentialvault"
	"applyflow/backend/internal/adapters/localfiles"
	"applyflow/backend/internal/adapters/postgres"
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/auth"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/transport/httpapi"
	"applyflow/backend/migrations"
	"github.com/pressly/goose/v3"
)

const testOrigin = "http://localhost:8080"

type browser struct {
	handler http.Handler
	cookies map[string]*http.Cookie
	csrf    string
}

func (b *browser) request(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, testOrigin+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	for _, cookie := range b.cookies {
		r.AddCookie(cookie)
	}
	if method != "GET" {
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("X-CSRF-Token", b.csrf)
		r.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	b.handler.ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(b.cookies, cookie.Name)
		} else {
			b.cookies[cookie.Name] = cookie
		}
	}
	return w
}
func requireStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("want HTTP %d, got %d: %s", status, w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response cached")
	}
}
func (b *browser) token(t *testing.T) {
	t.Helper()
	w := b.request("GET", "/api/auth/csrf", "", nil)
	requireStatus(t, w, 200)
	var data struct {
		Token string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	b.csrf = data.Token
}
func (b *browser) register(t *testing.T, email string) {
	t.Helper()
	b.token(t)
	w := b.request("POST", "/api/auth/register", `{"email":"`+email+`","password":"test-password-123"}`, nil)
	requireStatus(t, w, 201)
	b.token(t)
}
func newHTTP(t *testing.T) (http.Handler, *sql.DB, context.Context) {
	h, db, ctx, _ := newHTTPWithStorage(t)
	return h, db, ctx
}
func newHTTPWithStorage(t *testing.T) (http.Handler, *sql.DB, context.Context, *localfiles.Storage) {
	return newHTTPWithAI(t, testAIProbe(func(context.Context, string, []byte) aisettings.ProbeResult {
		return aisettings.ProbeResult{Status: "succeeded"}
	}))
}
func newHTTPWithAI(t *testing.T, probe aisettings.Probe) (http.Handler, *sql.DB, context.Context, *localfiles.Storage) {
	t.Helper()
	db, ctx := isolatedDatabase(t)
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := auth.New(postgres.AuthStore{DB: db}, security.Passwords{Cost: 4})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localfiles.Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = blobs.Close() })
	validator, err := localfiles.NewValidator("")
	if err != nil {
		t.Fatal("install Poppler for file integration tests:", err)
	}
	vault, err := credentialvault.New([]byte(strings.Repeat("k", 32)), 1)
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(a, studio.New(postgres.WorkspaceStore{DB: db}), httpapi.Options{AI: &aisettings.Service{Store: postgres.AISettingsStore{DB: db}, Vault: vault, Probe: probe}, Exports: postgres.FlowStore{DB: db}, Intake: &intake.Service{Store: postgres.FlowStore{DB: db}}, Generations: postgres.FlowStore{DB: db}, Documents: postgres.FlowStore{DB: db}, Tasks: postgres.FlowStore{DB: db}, Files: files.New(postgres.FileStore{DB: db}, blobs, validator), Resumes: resume.New(postgres.ResumeStore{DB: db}), Origin: testOrigin, SessionKey: []byte(strings.Repeat("s", 32)), CSRFKey: []byte(strings.Repeat("c", 32)), SessionTTL: 30 * time.Minute, Ready: db.PingContext, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return h, db, ctx, blobs
}
func newBrowser(h http.Handler) *browser {
	return &browser{handler: h, cookies: make(map[string]*http.Cookie)}
}
func TestHTTPAuthAndWorkspaceFlow(t *testing.T) {
	h, db, ctx := newHTTP(t)
	a, b := newBrowser(h), newBrowser(h)
	requireStatus(t, a.request("GET", "/api/workspaces", "", nil), 401)
	a.register(t, " Alice@Example.com ")
	b.register(t, "bob@example.com")
	if n := scalar(t, ctx, db, `SELECT count(*) FROM users u JOIN user_profiles p ON p.user_id=u.id WHERE u.email='alice@example.com'`); n != 1 {
		t.Fatal("registration did not atomically create profile")
	}
	cookie := a.cookies["applyflow_session"]
	if cookie == nil || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe session cookie")
	}
	w := a.request("GET", "/api/auth/me", "", nil)
	requireStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), cookie.Value) {
		t.Fatal("session response exposes secret")
	}
	key := security.UUID()
	w = a.request("POST", "/api/workspaces", `{"title":"Northstar"}`, map[string]string{"Idempotency-Key": key})
	requireStatus(t, w, 201)
	var doc struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), w.Body.Bytes()...)
	requireStatus(t, b.request("GET", "/api/workspaces/"+doc.ID, "", nil), 404)
	requireStatus(t, b.request("PATCH", "/api/workspaces/"+doc.ID, `{"expected_version":1,"title":"stolen"}`, nil), 404)
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+doc.ID, `{"expected_version":1,"title":"Edited"}`, nil), 200)
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+doc.ID, `{"expected_version":1,"title":"stale"}`, nil), 409)
	replay := a.request("POST", "/api/workspaces", `{"title":"Northstar"}`, map[string]string{"Idempotency-Key": key})
	requireStatus(t, replay, 201)
	if !bytes.Equal(original, replay.Body.Bytes()) {
		t.Fatal("idempotent replay changed after edit")
	}
	requireStatus(t, a.request("POST", "/api/workspaces", `{"title":"Different"}`, map[string]string{"Idempotency-Key": key}), 409)
	if n := scalar(t, ctx, db, `SELECT count(*) FROM workspaces`); n != 1 {
		t.Fatal("duplicate workspace")
	}
	w = b.request("GET", "/api/workspaces", "", nil)
	requireStatus(t, w, 200)
	if strings.Contains(w.Body.String(), doc.ID) {
		t.Fatal("list leaked workspace")
	}
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+doc.ID, `{"expected_version":2,"resume_revision_id":null}`, nil), 200)
	exec(t, ctx, db, `UPDATE workspaces SET archived_at=now() WHERE id=$1`, doc.ID)
	requireStatus(t, a.request("GET", "/api/workspaces/"+doc.ID, "", nil), 200)
	requireStatus(t, a.request("PATCH", "/api/workspaces/"+doc.ID, `{"expected_version":4,"title":"archived"}`, nil), 409)
	requireStatus(t, a.request("POST", "/api/auth/logout", "", nil), 204)
	requireStatus(t, a.request("POST", "/api/auth/logout", "", nil), 204)
	requireStatus(t, a.request("GET", "/api/auth/me", "", nil), 401)
	a.token(t)
	requireStatus(t, a.request("POST", "/api/auth/login", `{"email":"alice@example.com","password":"test-password-123"}`, nil), 200)
}
func TestHTTPSecurityInputAndPagination(t *testing.T) {
	h, _, _ := newHTTP(t)
	a, b := newBrowser(h), newBrowser(h)
	a.register(t, "alice@example.com")
	b.register(t, "bob@example.com")
	valid := `{"title":"Job"}`
	path := "/api/workspaces"
	requireStatus(t, a.request("POST", path, valid, map[string]string{"Origin": "https://evil.example", "Idempotency-Key": security.UUID()}), 403)
	requireStatus(t, a.request("POST", path, valid, map[string]string{"X-CSRF-Token": b.csrf, "Idempotency-Key": security.UUID()}), 403)
	for _, body := range []string{`{"title":"a","title":"b"}`, `{"owner_id":"injected"}`, `{"Title":"wrong case"}`, `{"title":null}`, `null`, `{} {}`, `{"title":" "}`} {
		requireStatus(t, a.request("POST", path, body, map[string]string{"Idempotency-Key": security.UUID()}), 400)
	}
	requireStatus(t, a.request("POST", path, valid, nil), 400)
	requireStatus(t, a.request("POST", path, valid, map[string]string{"Idempotency-Key": security.UUID(), "Content-Type": "text/plain"}), 415)
	requireStatus(t, a.request("POST", path, `{"title":"`+strings.Repeat("x", 131072)+`"}`, map[string]string{"Idempotency-Key": security.UUID()}), 413)
	for i := 0; i < 3; i++ {
		requireStatus(t, a.request("POST", path, valid, map[string]string{"Idempotency-Key": security.UUID()}), 201)
	}
	first := a.request("GET", path+"?limit=2", "", nil)
	requireStatus(t, first, 200)
	var page struct {
		Items []map[string]any `json:"items"`
		Next  *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Next == nil {
		t.Fatal("pagination boundary missing")
	}
	next := a.request("GET", path+"?limit=2&cursor="+*page.Next, "", nil)
	requireStatus(t, next, 200)
	var last struct {
		Items []map[string]any `json:"items"`
		Next  *string          `json:"next_cursor"`
	}
	json.Unmarshal(next.Body.Bytes(), &last)
	if len(last.Items) != 1 || last.Next != nil {
		t.Fatal("incorrect final page")
	}
	requireStatus(t, b.request("GET", path+"?cursor="+*page.Next, "", nil), 400)
	requireStatus(t, a.request("GET", path+"?cursor="+*page.Next+"tampered", "", nil), 400)
	requireStatus(t, a.request("GET", path+"?limit=0", "", nil), 400)
	requireStatus(t, a.request("GET", path+"?limit=1&limit=2", "", nil), 400)
	// A fresh login rotates the session binding and rejects the old CSRF pair.
	oldToken := a.csrf
	oldCookie := *a.cookies["applyflow_csrf"]
	requireStatus(t, a.request("POST", "/api/auth/login", `{"email":"alice@example.com","password":"test-password-123"}`, nil), 200)
	a.cookies["applyflow_csrf"] = &oldCookie
	a.csrf = oldToken
	requireStatus(t, a.request("POST", path, valid, map[string]string{"Idempotency-Key": security.UUID()}), 403)
}

func TestHTTPConcurrentCreateReplaysOneWorkspace(t *testing.T) {
	h, db, ctx := newHTTP(t)
	a := newBrowser(h)
	a.register(t, "concurrent@example.com")
	key := security.UUID()
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		client := newBrowser(h)
		client.csrf = a.csrf
		for name, cookie := range a.cookies {
			client.cookies[name] = cookie
		}
		go func(b *browser) {
			<-start
			responses <- b.request("POST", "/api/workspaces", `{"title":"Concurrent"}`, map[string]string{"Idempotency-Key": key})
		}(client)
	}
	close(start)
	first, second := <-responses, <-responses
	requireStatus(t, first, 201)
	requireStatus(t, second, 201)
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatal("parallel replay differs")
	}
	if n := scalar(t, ctx, db, `SELECT count(*) FROM workspaces`); n != 1 {
		t.Fatal("parallel requests created duplicates")
	}
	if n := scalar(t, ctx, db, `SELECT count(*) FROM idempotency_requests`); n != 1 {
		t.Fatal("duplicate replay record")
	}
}
