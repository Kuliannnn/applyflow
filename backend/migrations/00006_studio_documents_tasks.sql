-- +goose Up
CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, workspace_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('resume','cover_letter')),
    current_revision_id uuid,
    version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(id,workspace_id,owner_id), UNIQUE(id,kind,owner_id), UNIQUE(workspace_id,kind),
    FOREIGN KEY(workspace_id,owner_id) REFERENCES workspaces(id,owner_id)
);
CREATE TRIGGER documents_version BEFORE UPDATE ON documents FOR EACH ROW EXECUTE FUNCTION maintain_studio_version();
-- Run inputs are immutable and contain no credentials. This batch supports mock execution only.
CREATE TABLE generation_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, workspace_id uuid NOT NULL,
    job_revision_id uuid NOT NULL, resume_revision_id uuid NOT NULL,
    profile_version bigint NOT NULL CHECK (profile_version BETWEEN 1 AND 9007199254740991),
    profile_snapshot jsonb NOT NULL CHECK (jsonb_typeof(profile_snapshot)='object' AND octet_length(profile_snapshot::text)<=32768),
    execution_mode text NOT NULL DEFAULT 'mock' CHECK (execution_mode='mock'),
    locale text NOT NULL DEFAULT 'en' CHECK (locale='en'),
    prompt_version text NOT NULL CHECK (char_length(prompt_version) BETWEEN 1 AND 80),
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version=1),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(id,workspace_id,owner_id),
    FOREIGN KEY(workspace_id,owner_id) REFERENCES workspaces(id,owner_id),
    FOREIGN KEY(job_revision_id,workspace_id,owner_id) REFERENCES job_revisions(id,workspace_id,owner_id),
    FOREIGN KEY(resume_revision_id,owner_id) REFERENCES resume_revisions(id,owner_id)
);
CREATE TRIGGER generation_run_immutable BEFORE UPDATE ON generation_runs FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
ALTER TABLE workspaces ADD CONSTRAINT workspaces_run_fk
    FOREIGN KEY(current_run_id,id,owner_id) REFERENCES generation_runs(id,workspace_id,owner_id);
CREATE INDEX generation_runs_workspace ON generation_runs(owner_id,workspace_id,created_at DESC,id DESC);

CREATE TABLE job_tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id),
    workspace_id uuid, source_id uuid, resume_id uuid, document_id uuid, run_id uuid,
    kind text NOT NULL CHECK (kind IN ('extract_jd','parse_resume','tailor_resume','write_cover_letter','revise_document','export_document')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','retry_wait','completed','failed','cancelled')),
    stage text NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued','extracting','generating','validating','saving','exporting','retry_wait','completed','failed','cancelled')),
    state_version bigint NOT NULL DEFAULT 1 CHECK (state_version BETWEEN 1 AND 9007199254740991),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts>=0), max_attempts integer NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 5),
    fencing_token bigint NOT NULL DEFAULT 0 CHECK (fencing_token>=0),
    lease_until timestamptz, next_attempt_at timestamptz, cancel_requested boolean NOT NULL DEFAULT false,
    expected_document_version bigint CHECK (expected_document_version BETWEEN 1 AND 9007199254740991),
    input_snapshot jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(input_snapshot)='object' AND octet_length(input_snapshot::text)<=262144),
    result jsonb CHECK (jsonb_typeof(result)='object' AND octet_length(result::text)<=262144),
    safe_error_code text CHECK (char_length(safe_error_code) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz,
    UNIQUE(id,owner_id), UNIQUE(id,document_id,owner_id), UNIQUE(id,run_id,document_id,owner_id),
    FOREIGN KEY(workspace_id,owner_id) REFERENCES workspaces(id,owner_id),
    FOREIGN KEY(source_id,workspace_id,owner_id) REFERENCES job_sources(id,workspace_id,owner_id),
    FOREIGN KEY(resume_id,owner_id) REFERENCES resumes(id,owner_id),
    FOREIGN KEY(document_id,workspace_id,owner_id) REFERENCES documents(id,workspace_id,owner_id),
    FOREIGN KEY(run_id,workspace_id,owner_id) REFERENCES generation_runs(id,workspace_id,owner_id),
    CHECK (attempts<=max_attempts),
    CHECK (status='running' OR stage=status),
    CHECK (status='completed' OR result IS NULL),
    CHECK ((status='running')=(lease_until IS NOT NULL)),
    CHECK ((status='retry_wait')=(next_attempt_at IS NOT NULL)),
    CHECK ((status IN ('completed','failed','cancelled'))=(finished_at IS NOT NULL)),
    CHECK (status <> 'completed' OR (NOT cancel_requested AND result IS NOT NULL)),
    CHECK ((kind='extract_jd' AND workspace_id IS NOT NULL AND source_id IS NOT NULL AND resume_id IS NULL AND document_id IS NULL AND run_id IS NULL)
        OR (kind='parse_resume' AND workspace_id IS NULL AND source_id IS NULL AND resume_id IS NOT NULL AND document_id IS NULL AND run_id IS NULL)
        OR (kind IN ('tailor_resume','write_cover_letter','revise_document') AND workspace_id IS NOT NULL AND source_id IS NULL AND resume_id IS NULL AND document_id IS NOT NULL AND run_id IS NOT NULL AND expected_document_version IS NOT NULL)
        OR (kind='export_document' AND workspace_id IS NOT NULL AND source_id IS NULL AND resume_id IS NULL AND document_id IS NOT NULL AND run_id IS NULL))
);
CREATE UNIQUE INDEX tasks_active_document ON job_tasks(document_id)
    WHERE kind IN ('tailor_resume','write_cover_letter','revise_document') AND status IN ('queued','running','retry_wait');
CREATE UNIQUE INDEX tasks_active_source ON job_tasks(source_id) WHERE kind='extract_jd' AND status IN ('queued','running','retry_wait');
CREATE UNIQUE INDEX tasks_active_resume ON job_tasks(resume_id) WHERE kind='parse_resume' AND status IN ('queued','running','retry_wait');
CREATE INDEX tasks_owner_created ON job_tasks(owner_id,created_at DESC,id DESC);
CREATE INDEX tasks_recovery ON job_tasks(status,lease_until,next_attempt_at,created_at) WHERE status IN ('queued','running','retry_wait');

-- Enforce legal transitions, immutable targets, fencing and terminal protection.
-- Repository claim/complete statements must still use expected fence + lease predicates.
-- +goose StatementBegin
CREATE FUNCTION maintain_task_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status IN ('completed','failed','cancelled') THEN
        RAISE EXCEPTION 'terminal task is immutable' USING ERRCODE='23514';
    END IF;
    IF (NEW.id,NEW.owner_id,NEW.workspace_id,NEW.source_id,NEW.resume_id,NEW.document_id,NEW.run_id,NEW.kind,NEW.input_snapshot,NEW.expected_document_version,NEW.created_at,NEW.max_attempts)
       IS DISTINCT FROM (OLD.id,OLD.owner_id,OLD.workspace_id,OLD.source_id,OLD.resume_id,OLD.document_id,OLD.run_id,OLD.kind,OLD.input_snapshot,OLD.expected_document_version,OLD.created_at,OLD.max_attempts) THEN
        RAISE EXCEPTION 'task input is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.status<>OLD.status AND NOT (
       (OLD.status='queued' AND NEW.status IN ('running','cancelled','failed')) OR
       (OLD.status='running' AND NEW.status IN ('completed','retry_wait','failed','cancelled')) OR
       (OLD.status='retry_wait' AND NEW.status IN ('queued','cancelled','failed'))) THEN
        RAISE EXCEPTION 'invalid task transition' USING ERRCODE='23514';
    END IF;
    IF NEW.status='running' AND OLD.status='queued' THEN
        IF OLD.cancel_requested OR NEW.attempts<>OLD.attempts+1 OR NEW.fencing_token<>OLD.fencing_token+1
           OR NEW.lease_until<=clock_timestamp() THEN
            RAISE EXCEPTION 'invalid task claim' USING ERRCODE='23514';
        END IF;
    ELSIF NEW.attempts<>OLD.attempts OR NEW.fencing_token<>OLD.fencing_token THEN
        RAISE EXCEPTION 'attempt and fence change only on claim' USING ERRCODE='23514';
    END IF;
    IF OLD.status='running' AND NEW.status='running' AND OLD.lease_until<=clock_timestamp() THEN
        RAISE EXCEPTION 'expired lease cannot be renewed' USING ERRCODE='23514';
    END IF;
    IF NEW.status='completed' AND (OLD.cancel_requested OR OLD.lease_until<=clock_timestamp()) THEN
        RAISE EXCEPTION 'cancelled or expired claim cannot complete' USING ERRCODE='23514';
    END IF;
    IF OLD.cancel_requested AND NOT NEW.cancel_requested THEN
        RAISE EXCEPTION 'cannot clear cancellation' USING ERRCODE='23514';
    END IF;
    NEW.state_version := OLD.state_version + CASE WHEN
        (NEW.status,NEW.stage,NEW.cancel_requested,NEW.safe_error_code,NEW.result)
        IS DISTINCT FROM (OLD.status,OLD.stage,OLD.cancel_requested,OLD.safe_error_code,OLD.result) THEN 1 ELSE 0 END;
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER task_state BEFORE UPDATE ON job_tasks FOR EACH ROW EXECUTE FUNCTION maintain_task_state();

CREATE TABLE document_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, document_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('resume','cover_letter')),
    parent_revision_id uuid, run_id uuid NOT NULL, task_id uuid,
    origin text NOT NULL CHECK (origin IN ('ai','manual')),
    content jsonb NOT NULL CHECK (jsonb_typeof(content)='object' AND octet_length(content::text)<=262144),
    change_summary text NOT NULL DEFAULT '' CHECK (char_length(change_summary)<=2000),
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version=1),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(id,document_id,owner_id), UNIQUE(id,document_id,kind,owner_id), UNIQUE(task_id),
    FOREIGN KEY(document_id,kind,owner_id) REFERENCES documents(id,kind,owner_id),
    FOREIGN KEY(parent_revision_id,document_id,owner_id) REFERENCES document_revisions(id,document_id,owner_id),
    FOREIGN KEY(run_id,owner_id) REFERENCES generation_runs(id,owner_id),
    FOREIGN KEY(task_id,run_id,document_id,owner_id) REFERENCES job_tasks(id,run_id,document_id,owner_id),
    CHECK ((origin='ai' AND task_id IS NOT NULL) OR (origin='manual' AND task_id IS NULL)),
    CHECK (content ? 'kind' AND content->>'kind'=kind)
);
CREATE TRIGGER document_revision_immutable BEFORE UPDATE ON document_revisions FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
CREATE INDEX document_revisions_document ON document_revisions(owner_id,document_id,created_at DESC,id DESC);
ALTER TABLE documents ADD CONSTRAINT documents_head_fk
    FOREIGN KEY(current_revision_id,id,owner_id) REFERENCES document_revisions(id,document_id,owner_id);

CREATE TABLE task_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, task_id uuid NOT NULL,
    delivery_generation integer NOT NULL DEFAULT 1 CHECK (delivery_generation>0),
    claim_token uuid, claim_until timestamptz,
    next_dispatch_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(task_id,delivery_generation),
    FOREIGN KEY(task_id,owner_id) REFERENCES job_tasks(id,owner_id),
    CHECK ((claim_token IS NULL)=(claim_until IS NULL))
);
CREATE INDEX outbox_pending ON task_outbox(next_dispatch_at,created_at) WHERE published_at IS NULL;
CREATE TABLE document_exports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, document_id uuid NOT NULL,
    document_revision_id uuid NOT NULL, format text NOT NULL CHECK (format IN ('pdf','docx')),
    template_version text NOT NULL CHECK (char_length(template_version) BETWEEN 1 AND 80),
    task_id uuid NOT NULL UNIQUE, file_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(document_revision_id,format,template_version),
    FOREIGN KEY(document_revision_id,document_id,owner_id) REFERENCES document_revisions(id,document_id,owner_id),
    FOREIGN KEY(task_id,document_id,owner_id) REFERENCES job_tasks(id,document_id,owner_id),
    FOREIGN KEY(file_id,owner_id) REFERENCES files(id,owner_id)
);
CREATE TABLE application_documents (
    owner_id uuid NOT NULL, application_id uuid NOT NULL, document_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('resume','cover_letter')), document_revision_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(application_id,kind),
    FOREIGN KEY(application_id,owner_id) REFERENCES applications(id,owner_id),
    FOREIGN KEY(document_revision_id,document_id,kind,owner_id) REFERENCES document_revisions(id,document_id,kind,owner_id)
);
CREATE TABLE idempotency_requests (
    owner_id uuid NOT NULL REFERENCES users(id), operation text NOT NULL CHECK (char_length(operation) BETWEEN 1 AND 160),
    key_hash text NOT NULL CHECK (key_hash ~ '^[a-f0-9]{64}$'), request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
    response_status integer NOT NULL CHECK (response_status IN (200,201,202)),
    response_body jsonb NOT NULL CHECK (jsonb_typeof(response_body)='object' AND octet_length(response_body::text)<=32768),
    created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
    PRIMARY KEY(owner_id,operation,key_hash),
    CHECK (expires_at>=created_at+interval '24 hours')
);
CREATE INDEX idempotency_expiry ON idempotency_requests(expires_at);
COMMENT ON TABLE idempotency_requests IS 'Accepting transaction saves response + tasks + outbox atomically. Cleanup must also verify linked tasks are terminal; time alone is insufficient.';
COMMENT ON TABLE generation_runs IS 'Mock-only foundation. Live AI requires encrypted credentials, pinned revision FKs, usage reservations and a follow-up migration before enabling it.';
COMMENT ON TABLE document_revisions IS 'HTTP validates full content schema and fact provenance; a trigger validates the run workspace. Candidate persistence never changes document head without a CAS transaction.';

-- +goose StatementBegin
CREATE FUNCTION validate_studio_document_link() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE doc_workspace uuid; doc_kind text;
BEGIN
    IF TG_TABLE_NAME='job_tasks' THEN
        IF NEW.status<>'queued' OR NEW.stage<>'queued' OR NEW.attempts<>0 OR NEW.fencing_token<>0 OR NEW.state_version<>1 OR NEW.cancel_requested THEN
            RAISE EXCEPTION 'new task must start queued' USING ERRCODE='23514';
        END IF;
        IF NEW.kind IN ('tailor_resume','write_cover_letter') THEN
            SELECT kind INTO doc_kind FROM documents WHERE id=NEW.document_id AND owner_id=NEW.owner_id;
            IF doc_kind IS DISTINCT FROM (CASE WHEN NEW.kind='tailor_resume' THEN 'resume' ELSE 'cover_letter' END) THEN
                RAISE EXCEPTION 'task document kind mismatch' USING ERRCODE='23514';
            END IF;
        END IF;
    ELSIF TG_TABLE_NAME='document_revisions' THEN
        SELECT workspace_id INTO doc_workspace FROM documents WHERE id=NEW.document_id AND owner_id=NEW.owner_id;
        IF NOT EXISTS (SELECT 1 FROM generation_runs WHERE id=NEW.run_id AND owner_id=NEW.owner_id AND workspace_id=doc_workspace) THEN
            RAISE EXCEPTION 'revision run belongs to another workspace' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='documents' THEN
        IF (NEW.workspace_id,NEW.kind) IS DISTINCT FROM (OLD.workspace_id,OLD.kind) THEN
            RAISE EXCEPTION 'document identity is immutable' USING ERRCODE='23514';
        END IF;
    ELSIF TG_TABLE_NAME='document_exports' THEN
        IF TG_OP='UPDATE' AND (NEW.id,NEW.owner_id,NEW.document_id,NEW.document_revision_id,NEW.format,NEW.template_version,NEW.created_at)
          IS DISTINCT FROM (OLD.id,OLD.owner_id,OLD.document_id,OLD.document_revision_id,OLD.format,OLD.template_version,OLD.created_at) THEN
            RAISE EXCEPTION 'export target is immutable' USING ERRCODE='23514';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM job_tasks WHERE id=NEW.task_id AND owner_id=NEW.owner_id AND document_id=NEW.document_id AND kind='export_document') THEN
            RAISE EXCEPTION 'export needs export task' USING ERRCODE='23514';
        END IF;
        IF NEW.file_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM files WHERE id=NEW.file_id AND owner_id=NEW.owner_id AND purpose='export' AND state='ready'
          AND media_type=CASE WHEN NEW.format='pdf' THEN 'application/pdf' ELSE 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' END) THEN
            RAISE EXCEPTION 'export needs ready file of matching type' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER task_initial_state BEFORE INSERT ON job_tasks FOR EACH ROW EXECUTE FUNCTION validate_studio_document_link();
CREATE TRIGGER revision_run_validate BEFORE INSERT ON document_revisions FOR EACH ROW EXECUTE FUNCTION validate_studio_document_link();
CREATE TRIGGER document_identity BEFORE UPDATE ON documents FOR EACH ROW EXECUTE FUNCTION validate_studio_document_link();
CREATE TRIGGER export_validate BEFORE INSERT OR UPDATE ON document_exports FOR EACH ROW EXECUTE FUNCTION validate_studio_document_link();

-- +goose Down
ALTER TABLE documents DROP CONSTRAINT documents_head_fk;
ALTER TABLE workspaces DROP CONSTRAINT workspaces_run_fk;
DROP TABLE idempotency_requests, application_documents, document_exports, task_outbox, document_revisions, job_tasks, generation_runs, documents;
DROP FUNCTION validate_studio_document_link();
DROP FUNCTION maintain_task_state();
