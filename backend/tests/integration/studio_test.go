package integration

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"applyflow/backend/migrations"
	"github.com/pressly/goose/v3"
)

type studioFixture struct {
	db                                                          *sql.DB
	ctx                                                         context.Context
	workspace, resume, facts, source, jd, document, letter, run string
}

func rowID(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newStudio(t *testing.T) studioFixture {
	t.Helper()
	db, ctx := isolatedDatabase(t)
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	exec(t, ctx, db, "INSERT INTO users(id,email,password_hash) VALUES ($1,'alice@example.com',$3),($2,'bob@example.com',$3)", alice, bob, testHash)
	f := studioFixture{db: db, ctx: ctx}
	file := rowID(t, ctx, db, `INSERT INTO files(owner_id,purpose,state,original_name,storage_key,media_type,byte_size,sha256) VALUES ($1,'base_resume','ready','resume.pdf','private/base','application/pdf',100,repeat('a',64)) RETURNING id`, alice)
	f.resume = rowID(t, ctx, db, `INSERT INTO resumes(owner_id,name,source_file_id) VALUES ($1,'Base',$2) RETURNING id`, alice, file)
	f.facts = rowID(t, ctx, db, `INSERT INTO resume_revisions(owner_id,resume_id,facts) VALUES ($1,$2,'[{"id":"10000000-0000-4000-8000-000000000001","category":"experience","text":"Built APIs","evidence":{"source":"user","page":null,"excerpt":""}}]') RETURNING id`, alice, f.resume)
	exec(t, ctx, db, `UPDATE resumes SET current_revision_id=$2 WHERE id=$1`, f.resume, f.facts)
	f.workspace = rowID(t, ctx, db, `INSERT INTO workspaces(owner_id,resume_revision_id) VALUES ($1,$2) RETURNING id`, alice, f.facts)
	f.source = rowID(t, ctx, db, `INSERT INTO job_sources(owner_id,workspace_id,source_version,kind,raw_text) VALUES ($1,$2,1,'text','Go engineer') RETURNING id`, alice, f.workspace)
	f.jd = rowID(t, ctx, db, `INSERT INTO job_revisions(owner_id,workspace_id,source_id,company,role_title,job_description) VALUES ($1,$2,$3,'Northstar','Engineer','Go engineer') RETURNING id`, alice, f.workspace, f.source)
	exec(t, ctx, db, `UPDATE workspaces SET current_source_id=$2,job_revision_id=$3 WHERE id=$1`, f.workspace, f.source, f.jd)
	f.document = rowID(t, ctx, db, `INSERT INTO documents(owner_id,workspace_id,kind) VALUES ($1,$2,'resume') RETURNING id`, alice, f.workspace)
	f.letter = rowID(t, ctx, db, `INSERT INTO documents(owner_id,workspace_id,kind) VALUES ($1,$2,'cover_letter') RETURNING id`, alice, f.workspace)
	f.run = rowID(t, ctx, db, `INSERT INTO generation_runs(owner_id,workspace_id,job_revision_id,resume_revision_id,profile_version,profile_snapshot,prompt_version) VALUES ($1,$2,$3,$4,1,'{}','mock-1') RETURNING id`, alice, f.workspace, f.jd, f.facts)
	exec(t, ctx, db, `UPDATE workspaces SET current_run_id=$2 WHERE id=$1`, f.workspace, f.run)
	return f
}
func (f studioFixture) task(t *testing.T) string {
	t.Helper()
	return rowID(t, f.ctx, f.db, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,run_id,kind,expected_document_version) VALUES ($1,$2,$3,$4,'tailor_resume',1) RETURNING id`, alice, f.workspace, f.document, f.run)
}
func (f studioFixture) manual(t *testing.T, parent any) string {
	t.Helper()
	return rowID(t, f.ctx, f.db, `INSERT INTO document_revisions(owner_id,document_id,run_id,kind,origin,parent_revision_id,content) VALUES ($1,$2,$3,'resume','manual',$4,'{"kind":"resume","title":"Engineer","sections":[{"heading":"Experience","items":[{"text":"Built APIs","fact_ids":[]}]}]}') RETURNING id`, alice, f.document, f.run, parent)
}
func claimTask(t *testing.T, f studioFixture, id string) {
	t.Helper()
	exec(t, f.ctx, f.db, `UPDATE job_tasks SET status='running',stage='generating',attempts=attempts+1,fencing_token=fencing_token+1,lease_until=clock_timestamp()+interval '1 minute' WHERE id=$1 AND status='queued'`, id)
}

func TestStudioMigrations(t *testing.T) {
	t.Run("owned sources and confirmed revisions", func(t *testing.T) {
		f := newStudio(t)
		wantSQLState(t, f.ctx, f.db, "23503", `INSERT INTO workspaces(owner_id,resume_revision_id) VALUES ($1,$2)`, bob, f.facts)
		other := rowID(t, f.ctx, f.db, `INSERT INTO workspaces(owner_id) VALUES ($1) RETURNING id`, alice)
		wantSQLState(t, f.ctx, f.db, "23503", `UPDATE workspaces SET job_revision_id=$2 WHERE id=$1`, other, f.jd)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE resume_revisions SET facts='[]' WHERE id=$1`, f.facts)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_revisions SET company='Changed' WHERE id=$1`, f.jd)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE generation_runs SET prompt_version='new' WHERE id=$1`, f.run)
		wantSQLState(t, f.ctx, f.db, "23514", `INSERT INTO job_sources(owner_id,workspace_id,source_version,kind,raw_text,url) VALUES ($1,$2,2,'text','Go','https://example.com')`, alice, f.workspace)
		wantSQLState(t, f.ctx, f.db, "23514", `INSERT INTO job_sources(owner_id,workspace_id,source_version,kind,raw_text) VALUES ($1,$2,2,'text',repeat('界',21846))`, alice, f.workspace)
		pending := rowID(t, f.ctx, f.db, `INSERT INTO files(owner_id,purpose,original_name,storage_key,media_type,byte_size) VALUES ($1,'base_resume','p.pdf','pending','application/pdf',20) RETURNING id`, alice)
		wantSQLState(t, f.ctx, f.db, "23514", `INSERT INTO resumes(owner_id,name,source_file_id) VALUES ($1,'Pending',$2)`, alice, pending)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE files SET storage_key='changed' WHERE storage_key='private/base'`)
	})
	t.Run("one active task and immutable terminal state", func(t *testing.T) {
		f := newStudio(t)
		id := f.task(t)
		wantSQLState(t, f.ctx, f.db, "23505", `INSERT INTO job_tasks(owner_id,workspace_id,document_id,run_id,kind,expected_document_version) VALUES ($1,$2,$3,$4,'tailor_resume',1)`, alice, f.workspace, f.document, f.run)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_tasks SET status='completed',stage='completed',result='{}',finished_at=now() WHERE id=$1`, id)
		claimTask(t, f, id)
		if n := scalar(t, f.ctx, f.db, `SELECT state_version FROM job_tasks WHERE id=$1`, id); n != 2 {
			t.Fatal(n)
		}
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()+interval '2 minutes' WHERE id=$1`, id)
		if n := scalar(t, f.ctx, f.db, `SELECT state_version FROM job_tasks WHERE id=$1`, id); n != 2 {
			t.Fatal("heartbeat changed visible version")
		}
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET status='completed',stage='completed',result='{}',lease_until=NULL,finished_at=now() WHERE id=$1 AND status='running' AND fencing_token=1 AND lease_until>clock_timestamp() AND NOT cancel_requested`, id)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_tasks SET safe_error_code='late' WHERE id=$1`, id)
		_ = f.task(t) // Deliberate rerun is allowed once the prior task is terminal.
	})
	t.Run("cancel and expired fence cannot commit", func(t *testing.T) {
		f := newStudio(t)
		id := f.task(t)
		claimTask(t, f, id)
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET cancel_requested=true WHERE id=$1`, id)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_tasks SET status='completed',stage='completed',result='{}',lease_until=NULL,finished_at=now() WHERE id=$1`, id)
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET status='cancelled',stage='cancelled',lease_until=NULL,finished_at=now() WHERE id=$1`, id)
		next := f.task(t)
		claimTask(t, f, next)
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, next)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_tasks SET lease_until=clock_timestamp()+interval '1 minute' WHERE id=$1`, next)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE job_tasks SET status='completed',stage='completed',result='{}',lease_until=NULL,finished_at=now() WHERE id=$1`, next)
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET status='retry_wait',stage='retry_wait',lease_until=NULL,next_attempt_at=now() WHERE id=$1`, next)
		exec(t, f.ctx, f.db, `UPDATE job_tasks SET status='queued',stage='queued',next_attempt_at=NULL WHERE id=$1`, next)
		claimTask(t, f, next)
		if n := scalar(t, f.ctx, f.db, `SELECT fencing_token FROM job_tasks WHERE id=$1`, next); n != 2 {
			t.Fatal(n)
		}
		result, err := f.db.ExecContext(f.ctx, `UPDATE job_tasks SET status='completed',stage='completed',result='{}',lease_until=NULL,finished_at=now() WHERE id=$1 AND status='running' AND fencing_token=1 AND lease_until>clock_timestamp() AND NOT cancel_requested`, next)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := result.RowsAffected()
		if n != 0 {
			t.Fatal("stale worker committed")
		}
	})
	t.Run("candidate cannot overwrite edited head", func(t *testing.T) {
		f := newStudio(t)
		first := f.manual(t, nil)
		exec(t, f.ctx, f.db, `UPDATE documents SET current_revision_id=$2 WHERE id=$1 AND owner_id=$3 AND version=1`, f.document, first, alice)
		second := f.manual(t, first)
		total := raceWrites(t, f.ctx, f.db, `UPDATE documents SET current_revision_id=$2 WHERE id=$1 AND owner_id=$3 AND version=2`, f.document, second, alice)
		if total != 1 {
			t.Fatalf("CAS winners=%d", total)
		}
		r, err := f.db.ExecContext(f.ctx, `UPDATE documents SET current_revision_id=$2 WHERE id=$1 AND owner_id=$3 AND version=1`, f.document, first, alice)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := r.RowsAffected()
		if n != 0 {
			t.Fatal("late result overwrote edit")
		}
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE document_revisions SET change_summary='rewrite' WHERE id=$1`, first)
		wantSQLState(t, f.ctx, f.db, "23503", `UPDATE documents SET current_revision_id=$2 WHERE id=$1`, f.letter, first)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE documents SET kind='cover_letter' WHERE id=$1`, f.document)
	})
	t.Run("failed acceptance rolls back run tasks and outbox", func(t *testing.T) {
		f := newStudio(t)
		existing := f.task(t)
		tx, err := f.db.BeginTx(f.ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var run, task string
		err = tx.QueryRowContext(f.ctx, `INSERT INTO generation_runs(owner_id,workspace_id,job_revision_id,resume_revision_id,profile_version,profile_snapshot,prompt_version) VALUES ($1,$2,$3,$4,1,'{}','mock-2') RETURNING id`, alice, f.workspace, f.jd, f.facts).Scan(&run)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.QueryRowContext(f.ctx, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,run_id,kind,expected_document_version) VALUES ($1,$2,$3,$4,'write_cover_letter',1) RETURNING id`, alice, f.workspace, f.letter, run).Scan(&task)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(f.ctx, `INSERT INTO task_outbox(owner_id,task_id) VALUES ($1,$2)`, alice, task); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(f.ctx, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,run_id,kind,expected_document_version) SELECT owner_id,workspace_id,document_id,$2,kind,expected_document_version FROM job_tasks WHERE id=$1`, existing, run); err == nil {
			t.Fatal("duplicate active task accepted")
		}
		if err = tx.Commit(); err == nil {
			t.Fatal("partial acceptance committed")
		}
		if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM generation_runs`); n != 1 {
			t.Fatal(n)
		}
		if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM job_tasks`); n != 1 {
			t.Fatal(n)
		}
		if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM task_outbox`); n != 0 {
			t.Fatal(n)
		}
	})
	t.Run("idempotency key is unique per owner and operation", func(t *testing.T) {
		f := newStudio(t)
		q := `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES ($1,'generate',repeat('a',64),repeat('b',64),202,'{}',now()+interval '25 hours') ON CONFLICT DO NOTHING`
		if n := raceWrites(t, f.ctx, f.db, q, alice); n != 1 {
			t.Fatalf("idempotency winners=%d", n)
		}
		exec(t, f.ctx, f.db, q, bob)
		wantSQLState(t, f.ctx, f.db, "23505", `INSERT INTO idempotency_requests(owner_id,operation,key_hash,request_hash,response_status,response_body,expires_at) VALUES ($1,'generate',repeat('a',64),repeat('c',64),202,'{}',now()+interval '25 hours')`, alice)
		if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM idempotency_requests WHERE owner_id=$1 AND request_hash=repeat('b',64)`, alice); n != 1 {
			t.Fatal("replay body changed")
		}
	})
	t.Run("exports pin immutable owned revision", func(t *testing.T) {
		f := newStudio(t)
		rev := f.manual(t, nil)
		task := rowID(t, f.ctx, f.db, `INSERT INTO job_tasks(owner_id,workspace_id,document_id,kind) VALUES ($1,$2,$3,'export_document') RETURNING id`, alice, f.workspace, f.document)
		id := rowID(t, f.ctx, f.db, `INSERT INTO document_exports(owner_id,document_id,document_revision_id,format,template_version,task_id) VALUES ($1,$2,$3,'pdf','1',$4) RETURNING id`, alice, f.document, rev, task)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE document_exports SET format='docx' WHERE id=$1`, id)
		wantSQLState(t, f.ctx, f.db, "23514", `UPDATE document_exports SET file_id=(SELECT source_file_id FROM resumes WHERE id=$2) WHERE id=$1`, id, f.resume)
		wantSQLState(t, f.ctx, f.db, "23503", `INSERT INTO document_revisions(owner_id,document_id,run_id,kind,origin,content) VALUES ($1,$2,$3,'cover_letter','manual','{"kind":"cover_letter"}')`, alice, f.document, f.run)
		if n := scalar(t, f.ctx, f.db, `SELECT count(*) FROM document_exports WHERE owner_id=$1`, bob); n != 0 {
			t.Fatal("owner filter exposed export")
		}
	})
}

// A barrier starts genuinely concurrent database statements; no sleeps or mock locks.
func raceWrites(t *testing.T, ctx context.Context, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	start := make(chan struct{})
	counts := make(chan int64, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := db.ExecContext(ctx, q, args...)
			if err != nil {
				failures <- err
				return
			}
			n, err := r.RowsAffected()
			if err != nil {
				failures <- err
				return
			}
			counts <- n
		}()
	}
	close(start)
	wg.Wait()
	close(counts)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var total int64
	for n := range counts {
		total += n
	}
	return total
}

func TestStudioUpgradePreservesFoundation(t *testing.T) {
	db, ctx := isolatedDatabase(t)
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	exec(t, ctx, db, `INSERT INTO users(id,email,password_hash) VALUES ($1,'alice@example.com',$2)`, alice, testHash)
	exec(t, ctx, db, `INSERT INTO applications(id,owner_id,company,role_title) VALUES ($1,$2,'Preserve','Engineer')`, job, alice)
	r, err := p.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(r) != 2 {
		t.Fatalf("upgrade count=%d", len(r))
	}
	if n := scalar(t, ctx, db, `SELECT count(*) FROM applications WHERE id=$1 AND company='Preserve' AND version=1`, job); n != 1 {
		t.Fatal("foundation data changed")
	}
	if _, err = p.DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, ctx, db, `SELECT count(*) FROM applications WHERE id=$1`, job); n != 1 {
		t.Fatal("down damaged foundation")
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}
