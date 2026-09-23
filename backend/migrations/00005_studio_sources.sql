-- +goose Up
-- Metadata and confirmed revisions only. File IO/parsing are application concerns.
CREATE TABLE files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id),
    purpose text NOT NULL CHECK (purpose IN ('jd_image','base_resume','export')),
    state text NOT NULL DEFAULT 'uploading' CHECK (state IN ('uploading','ready','rejected')),
    original_name text NOT NULL CHECK (char_length(original_name) BETWEEN 1 AND 255),
    storage_key text NOT NULL UNIQUE CHECK (char_length(storage_key) BETWEEN 1 AND 1024),
    media_type text NOT NULL CHECK (media_type IN ('image/png','image/jpeg','image/webp','application/pdf','application/vnd.openxmlformats-officedocument.wordprocessingml.document')),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 10485760),
    sha256 text CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id, owner_id),
    CHECK (state <> 'ready' OR sha256 IS NOT NULL),
    CHECK ((purpose='jd_image' AND media_type LIKE 'image/%') OR
           (purpose IN ('base_resume','export') AND media_type IN ('application/pdf','application/vnd.openxmlformats-officedocument.wordprocessingml.document')))
);
CREATE INDEX files_owner_created ON files(owner_id, created_at DESC, id DESC);

CREATE TABLE resumes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 160),
    source_file_id uuid NOT NULL,
    current_revision_id uuid,
    is_default boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id, owner_id),
    FOREIGN KEY(source_file_id, owner_id) REFERENCES files(id, owner_id)
);
CREATE UNIQUE INDEX resumes_one_default ON resumes(owner_id) WHERE is_default;
CREATE INDEX resumes_owner_updated ON resumes(owner_id, updated_at DESC, id DESC);
CREATE TABLE resume_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL,
    resume_id uuid NOT NULL, parent_revision_id uuid,
    facts jsonb NOT NULL CHECK (jsonb_typeof(facts)='array' AND jsonb_array_length(facts) BETWEEN 1 AND 200),
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version=1),
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id, owner_id), UNIQUE(id, resume_id, owner_id),
    FOREIGN KEY(resume_id, owner_id) REFERENCES resumes(id, owner_id),
    FOREIGN KEY(parent_revision_id, resume_id, owner_id) REFERENCES resume_revisions(id, resume_id, owner_id),
    CHECK (octet_length(facts::text)<=262144)
);
ALTER TABLE resumes ADD CONSTRAINT resumes_current_revision_fk
    FOREIGN KEY(current_revision_id,id,owner_id) REFERENCES resume_revisions(id,resume_id,owner_id);
CREATE INDEX resume_revisions_parent ON resume_revisions(owner_id,resume_id,confirmed_at DESC,id DESC);

CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id),
    title text NOT NULL DEFAULT 'Untitled application' CHECK (char_length(btrim(title)) BETWEEN 1 AND 160),
    version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991),
    current_source_id uuid, job_revision_id uuid, resume_revision_id uuid, current_run_id uuid,
    application_id uuid, archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(application_id),
    FOREIGN KEY(resume_revision_id,owner_id) REFERENCES resume_revisions(id,owner_id),
    FOREIGN KEY(application_id,owner_id) REFERENCES applications(id,owner_id)
);
CREATE INDEX workspaces_owner_updated ON workspaces(owner_id,updated_at DESC,id DESC) WHERE archived_at IS NULL;
CREATE TABLE job_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, workspace_id uuid NOT NULL,
    source_version bigint NOT NULL CHECK (source_version BETWEEN 1 AND 9007199254740991),
    kind text NOT NULL CHECK (kind IN ('text','url','image')),
    raw_text text, url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(id,workspace_id,owner_id), UNIQUE(workspace_id,source_version),
    FOREIGN KEY(workspace_id,owner_id) REFERENCES workspaces(id,owner_id),
    CHECK ((kind='text' AND raw_text IS NOT NULL AND char_length(btrim(raw_text))>0 AND octet_length(raw_text)<=65536 AND url IS NULL)
        OR (kind='url' AND raw_text IS NULL AND url IS NOT NULL AND char_length(url)<=2048 AND url ~ '^https://[^[:space:]]+$')
        OR (kind='image' AND raw_text IS NULL AND url IS NULL))
);
CREATE TABLE job_source_files (
    source_id uuid NOT NULL, owner_id uuid NOT NULL, file_id uuid NOT NULL,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 4),
    PRIMARY KEY(source_id,position), UNIQUE(source_id,file_id),
    FOREIGN KEY(source_id,owner_id) REFERENCES job_sources(id,owner_id),
    FOREIGN KEY(file_id,owner_id) REFERENCES files(id,owner_id)
);
CREATE TABLE job_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL, workspace_id uuid NOT NULL,
    source_id uuid NOT NULL,
    company text NOT NULL CHECK (char_length(btrim(company)) BETWEEN 1 AND 200),
    role_title text NOT NULL CHECK (char_length(btrim(role_title)) BETWEEN 1 AND 200),
    job_description text NOT NULL CHECK (char_length(btrim(job_description))>0 AND octet_length(job_description)<=65536),
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,owner_id), UNIQUE(id,workspace_id,owner_id),
    FOREIGN KEY(source_id,workspace_id,owner_id) REFERENCES job_sources(id,workspace_id,owner_id)
);
ALTER TABLE workspaces ADD CONSTRAINT workspaces_source_fk
    FOREIGN KEY(current_source_id,id,owner_id) REFERENCES job_sources(id,workspace_id,owner_id);
ALTER TABLE workspaces ADD CONSTRAINT workspaces_job_revision_fk
    FOREIGN KEY(job_revision_id,id,owner_id) REFERENCES job_revisions(id,workspace_id,owner_id);
CREATE INDEX job_revisions_workspace ON job_revisions(owner_id,workspace_id,confirmed_at DESC,id DESC);

-- Shared by mutable aggregates; repositories still provide owner/version predicates.
-- +goose StatementBegin
CREATE FUNCTION maintain_studio_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'studio identity is immutable' USING ERRCODE='23514';
    END IF;
    NEW.version := OLD.version + 1;
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER resumes_version BEFORE UPDATE ON resumes FOR EACH ROW EXECUTE FUNCTION maintain_studio_version();
CREATE TRIGGER workspaces_version BEFORE UPDATE ON workspaces FOR EACH ROW EXECUTE FUNCTION maintain_studio_version();
-- +goose StatementBegin
CREATE FUNCTION reject_revision_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'revision is immutable; insert a new revision' USING ERRCODE='23514';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER resume_revision_immutable BEFORE UPDATE ON resume_revisions FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
CREATE TRIGGER job_revision_immutable BEFORE UPDATE ON job_revisions FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
CREATE TRIGGER job_source_immutable BEFORE UPDATE ON job_sources FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
CREATE TRIGGER job_source_file_immutable BEFORE UPDATE ON job_source_files FOR EACH ROW EXECUTE FUNCTION reject_revision_update();
-- +goose StatementBegin
CREATE FUNCTION protect_ready_file() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state <> 'uploading' OR NEW.id<>OLD.id OR NEW.owner_id<>OLD.owner_id
       OR NEW.purpose<>OLD.purpose OR NEW.storage_key<>OLD.storage_key
       OR NEW.created_at<>OLD.created_at THEN
        RAISE EXCEPTION 'file identity or finalized content is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER files_protect BEFORE UPDATE ON files FOR EACH ROW EXECUTE FUNCTION protect_ready_file();
COMMENT ON TABLE resume_revisions IS 'Confirmed facts only. Handler validates the complete OpenAPI fact schema and unique fact IDs. No automatic proficiency inference.';
COMMENT ON TABLE job_sources IS 'Immutable submitted input. Image sources require 1-5 ready owned jd_image files verified in the accepting transaction. URL validation/SSRF controls are required outside SQL.';

-- +goose StatementBegin
CREATE FUNCTION validate_studio_source_file() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME='resumes' THEN
        IF TG_OP='UPDATE' AND NEW.source_file_id IS DISTINCT FROM OLD.source_file_id THEN
            RAISE EXCEPTION 'base file is immutable' USING ERRCODE='23514';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM files WHERE id=NEW.source_file_id AND owner_id=NEW.owner_id AND purpose='base_resume' AND state='ready') THEN
            RAISE EXCEPTION 'base resume requires a ready owned file' USING ERRCODE='23514';
        END IF;
    ELSE
        IF NOT EXISTS (SELECT 1 FROM files WHERE id=NEW.file_id AND owner_id=NEW.owner_id AND purpose='jd_image' AND state='ready')
           OR NOT EXISTS (SELECT 1 FROM job_sources WHERE id=NEW.source_id AND owner_id=NEW.owner_id AND kind='image') THEN
            RAISE EXCEPTION 'screenshot requires image source and ready owned image' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER resume_file_validate BEFORE INSERT OR UPDATE ON resumes FOR EACH ROW EXECUTE FUNCTION validate_studio_source_file();
CREATE TRIGGER source_file_validate BEFORE INSERT ON job_source_files FOR EACH ROW EXECUTE FUNCTION validate_studio_source_file();

-- +goose Down
ALTER TABLE workspaces DROP CONSTRAINT workspaces_job_revision_fk;
ALTER TABLE workspaces DROP CONSTRAINT workspaces_source_fk;
ALTER TABLE resumes DROP CONSTRAINT resumes_current_revision_fk;
DROP TABLE job_revisions, job_source_files, job_sources, workspaces, resume_revisions, resumes, files;
DROP FUNCTION validate_studio_source_file();
DROP FUNCTION protect_ready_file();
DROP FUNCTION reject_revision_update();
DROP FUNCTION maintain_studio_version();
