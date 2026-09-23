-- +goose Up
-- A text domain keeps status values consistent across applications and history.
CREATE DOMAIN application_status AS text CHECK (VALUE IN (
    'saved', 'applied', 'hr_interview', 'technical_interview',
    'final_interview', 'offer', 'rejected', 'withdrawn'
));

CREATE TABLE applications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    company text NOT NULL CHECK (char_length(btrim(company)) BETWEEN 1 AND 200),
    role_title text NOT NULL CHECK (char_length(btrim(role_title)) BETWEEN 1 AND 200),
    location text CHECK (char_length(location) BETWEEN 1 AND 200),
    job_url text CHECK (char_length(job_url) <= 2048 AND job_url ~ '^https://[^[:space:]]+$'),
    job_description text NOT NULL DEFAULT '' CHECK (octet_length(job_description) <= 65536),
    status application_status NOT NULL DEFAULT 'saved',
    source text CHECK (char_length(source) BETWEEN 1 AND 100),
    salary text CHECK (char_length(salary) BETWEEN 1 AND 200),
    date_applied date,
    first_response_at timestamptz,
    notes text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 10000),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    jd_version bigint NOT NULL DEFAULT 1 CHECK (jd_version > 0),
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT applications_id_owner_unique UNIQUE (id, owner_id)
);
CREATE INDEX applications_owner_updated ON applications(owner_id, updated_at DESC, id DESC);
CREATE INDEX applications_owner_status_updated ON applications(owner_id, status, updated_at DESC, id DESC)
    WHERE archived_at IS NULL;

CREATE TABLE application_status_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    from_status application_status,
    to_status application_status NOT NULL,
    application_version bigint NOT NULL CHECK (application_version > 0),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT status_history_application_owner_fk FOREIGN KEY (application_id, owner_id)
        REFERENCES applications(id, owner_id) ON DELETE CASCADE,
    CONSTRAINT status_history_version_unique UNIQUE (application_id, application_version),
    CONSTRAINT status_history_changed CHECK (from_status IS NULL OR from_status <> to_status)
);
CREATE INDEX status_history_owner_application ON application_status_history(owner_id, application_id, application_version);

-- Every update advances the optimistic version; JD changes advance its own version.
-- Handler/repository must still use WHERE owner_id=? AND version=? for authorization/CAS.
-- +goose StatementBegin
CREATE FUNCTION maintain_application_versions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'application identity is immutable' USING ERRCODE = '23514';
    END IF;
    NEW.version := OLD.version + 1;
    NEW.jd_version := OLD.jd_version + CASE WHEN NEW.job_description IS DISTINCT FROM OLD.job_description THEN 1 ELSE 0 END;
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER applications_maintain_versions BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION maintain_application_versions();

-- History and current status cannot partially commit, even if a future writer forgets history.
-- +goose StatementBegin
CREATE FUNCTION record_application_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO application_status_history(application_id, owner_id, from_status, to_status, application_version)
        VALUES (NEW.id, NEW.owner_id, NULL, NEW.status, NEW.version);
    ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO application_status_history(application_id, owner_id, from_status, to_status, application_version)
        VALUES (NEW.id, NEW.owner_id, OLD.status, NEW.status, NEW.version);
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER applications_record_status AFTER INSERT OR UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION record_application_status();
COMMENT ON TABLE application_status_history IS
    'Written by the application trigger. Repositories must not write history directly. This invoker trigger requires runtime INSERT privileges on history; timestamps represent recorded changes, not inferred past events.';

-- +goose Down
DROP TRIGGER applications_record_status ON applications;
DROP TRIGGER applications_maintain_versions ON applications;
DROP FUNCTION record_application_status();
DROP FUNCTION maintain_application_versions();
DROP TABLE application_status_history;
DROP TABLE applications;
DROP DOMAIN application_status;
