-- +goose Up
-- Secrets are encrypted outside PostgreSQL; no request body/key is stored in test receipts.
CREATE TABLE ai_credentials (
 id uuid PRIMARY KEY, owner_id uuid NOT NULL REFERENCES users(id),
 ciphertext bytea, nonce bytea, key_version integer NOT NULL CHECK(key_version>0),
 revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,owner_id),
 CHECK ((revoked_at IS NULL AND ciphertext IS NOT NULL AND octet_length(ciphertext) BETWEEN 17 AND 4112 AND nonce IS NOT NULL AND octet_length(nonce)=12)
 OR (revoked_at IS NOT NULL AND ciphertext IS NULL AND nonce IS NULL))
);
CREATE TABLE user_ai_config_revisions (
 id uuid PRIMARY KEY, owner_id uuid NOT NULL REFERENCES users(id),
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 9007199254740991),
 provider_id text NOT NULL CHECK(provider_id='openai'),
 model_id text NOT NULL CHECK(model_id IN ('gpt-4.1-mini','gpt-4.1')),
 credential_id uuid NOT NULL,
 test_status text NOT NULL DEFAULT 'untested' CHECK(test_status IN ('untested','testing','succeeded','failed','inconclusive')),
 test_run_id uuid, test_deadline timestamptz, last_tested_at timestamptz,
 error_code text CHECK(error_code IN ('provider_auth_failed','provider_model_unavailable','provider_rate_limited','provider_unavailable','provider_invalid_response','provider_timeout','credential_unavailable','test_interrupted')),
 revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,owner_id), UNIQUE(owner_id,revision),
 FOREIGN KEY(credential_id,owner_id) REFERENCES ai_credentials(id,owner_id),
 CHECK((test_status='testing')=(test_deadline IS NOT NULL)),
 CHECK(test_status<>'testing' OR test_run_id IS NOT NULL)
);
-- +goose StatementBegin
CREATE FUNCTION protect_ai_revision_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.id,NEW.owner_id,NEW.revision,NEW.provider_id,NEW.model_id,NEW.credential_id,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.id,OLD.owner_id,OLD.revision,OLD.provider_id,OLD.model_id,OLD.credential_id,OLD.created_at) THEN
  RAISE EXCEPTION 'AI revision identity is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN
  RAISE EXCEPTION 'AI revision revocation is permanent' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER ai_revision_identity BEFORE UPDATE ON user_ai_config_revisions FOR EACH ROW EXECUTE FUNCTION protect_ai_revision_identity();
CREATE TABLE user_ai_settings (
 user_id uuid PRIMARY KEY REFERENCES users(id),
 version bigint NOT NULL DEFAULT 0 CHECK(version BETWEEN 0 AND 9007199254740991),
 revision_counter bigint NOT NULL DEFAULT 0 CHECK(revision_counter BETWEEN 0 AND 9007199254740991),
 current_revision_id uuid, enabled boolean NOT NULL DEFAULT false,
 daily_request_limit integer NOT NULL DEFAULT 20 CHECK(daily_request_limit BETWEEN 1 AND 100),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(current_revision_id,user_id) REFERENCES user_ai_config_revisions(id,owner_id),
 CHECK(NOT enabled OR current_revision_id IS NOT NULL)
);
CREATE TABLE ai_test_requests (
 owner_id uuid NOT NULL REFERENCES users(id), key_hash text NOT NULL CHECK(length(key_hash)=64),
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 revision_id uuid NOT NULL, run_id uuid NOT NULL UNIQUE,
 state text NOT NULL CHECK(state IN ('running','done')), safe_result jsonb,
 deadline timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id,key_hash), FOREIGN KEY(revision_id,owner_id) REFERENCES user_ai_config_revisions(id,owner_id),
 CHECK((state='done')=(safe_result IS NOT NULL)),
 CHECK(safe_result IS NULL OR (jsonb_typeof(safe_result)='object' AND octet_length(safe_result::text)<=4096))
);
CREATE UNIQUE INDEX ai_one_test_per_owner ON ai_test_requests(owner_id) WHERE state='running';
CREATE INDEX ai_test_rate_budget ON ai_test_requests(owner_id,created_at);

-- +goose Down
DROP TABLE ai_test_requests;
DROP TABLE user_ai_settings;
DROP TABLE user_ai_config_revisions;
DROP FUNCTION protect_ai_revision_identity();
DROP TABLE ai_credentials;
