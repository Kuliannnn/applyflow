-- +goose Up
-- A full JD is sufficient input. Empty metadata is unknown, never fabricated.
ALTER TABLE job_revisions
 DROP CONSTRAINT job_revisions_company_check,
 DROP CONSTRAINT job_revisions_role_title_check,
 ADD CONSTRAINT job_revisions_company_check CHECK (company = '' OR char_length(btrim(company)) BETWEEN 1 AND 200),
 ADD CONSTRAINT job_revisions_role_title_check CHECK (role_title = '' OR char_length(btrim(role_title)) BETWEEN 1 AND 200);

-- +goose Down
-- Refuse rollback if unknown metadata exists; never rewrite immutable snapshots.
ALTER TABLE job_revisions
 DROP CONSTRAINT job_revisions_company_check,
 DROP CONSTRAINT job_revisions_role_title_check,
 ADD CONSTRAINT job_revisions_company_check CHECK (char_length(btrim(company)) BETWEEN 1 AND 200),
 ADD CONSTRAINT job_revisions_role_title_check CHECK (char_length(btrim(role_title)) BETWEEN 1 AND 200);
