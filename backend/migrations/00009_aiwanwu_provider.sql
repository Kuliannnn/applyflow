-- +goose Up
ALTER TABLE user_ai_config_revisions
 DROP CONSTRAINT user_ai_config_revisions_provider_id_check,
 DROP CONSTRAINT user_ai_config_revisions_model_id_check,
 ADD CONSTRAINT ai_provider_model_check CHECK (
  (provider_id='openai' AND model_id IN ('gpt-4.1-mini','gpt-4.1')) OR
  (provider_id='aiwanwu' AND model_id='gpt-6-sol')
 );

-- +goose Down
-- Refuse rollback while relay revisions exist; never delete credentials/history.
ALTER TABLE user_ai_config_revisions
 DROP CONSTRAINT ai_provider_model_check,
 ADD CONSTRAINT user_ai_config_revisions_provider_id_check CHECK(provider_id='openai'),
 ADD CONSTRAINT user_ai_config_revisions_model_id_check CHECK(model_id IN ('gpt-4.1-mini','gpt-4.1'));
