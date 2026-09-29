-- +goose Up
ALTER TABLE generation_runs DROP CONSTRAINT generation_runs_execution_mode_check;
ALTER TABLE generation_runs ADD CONSTRAINT generation_runs_execution_mode_check CHECK(execution_mode IN ('mock','personal'));
ALTER TABLE generation_runs ADD COLUMN ai_revision_id uuid;
ALTER TABLE generation_runs ADD CONSTRAINT generation_ai_revision_owner FOREIGN KEY(ai_revision_id,owner_id) REFERENCES user_ai_config_revisions(id,owner_id);
ALTER TABLE generation_runs ADD CONSTRAINT generation_ai_mode CHECK((execution_mode='personal')=(ai_revision_id IS NOT NULL));
CREATE TABLE ai_generation_usage (
 task_id uuid PRIMARY KEY, owner_id uuid NOT NULL, revision_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','started','completed','failed','unknown')),
 created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz,
 input_tokens bigint CHECK(input_tokens>=0), output_tokens bigint CHECK(output_tokens>=0),
 FOREIGN KEY(task_id,owner_id) REFERENCES job_tasks(id,owner_id),
 FOREIGN KEY(revision_id,owner_id) REFERENCES user_ai_config_revisions(id,owner_id),
 CHECK((state='reserved')=(started_at IS NULL)),
 CHECK((input_tokens IS NULL)=(output_tokens IS NULL))
);
CREATE INDEX ai_generation_owner_budget ON ai_generation_usage(owner_id,created_at);
-- +goose Down
DROP TABLE ai_generation_usage;
-- Refuse down-migration with live runs; removing the run binding would lose provenance.
ALTER TABLE generation_runs DROP CONSTRAINT generation_ai_mode;
ALTER TABLE generation_runs DROP CONSTRAINT generation_ai_revision_owner;
ALTER TABLE generation_runs DROP COLUMN ai_revision_id;
ALTER TABLE generation_runs DROP CONSTRAINT generation_runs_execution_mode_check;
ALTER TABLE generation_runs ADD CONSTRAINT generation_runs_execution_mode_check CHECK(execution_mode='mock');
