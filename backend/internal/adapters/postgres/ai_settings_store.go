package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
)

type AISettingsStore struct{ DB *sql.DB }

// Every settings transaction takes the user lock first. Provider I/O always
// happens after commit, so unrelated account work never waits on the network.
func (s AISettingsStore) beginAI(ctx context.Context, owner string) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*sql.Tx, error) { _ = tx.Rollback(); return nil, err }
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&id); err != nil {
		return fail(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_ai_settings(user_id) VALUES($1) ON CONFLICT DO NOTHING`, owner); err != nil {
		return fail(err)
	}
	if err = expireAITests(ctx, tx, owner); err != nil {
		return fail(err)
	}
	return tx, nil
}
func readAI(ctx context.Context, tx *sql.Tx, owner string) (aisettings.Config, error) {
	out := aisettings.Empty()
	err := tx.QueryRowContext(ctx, `SELECT s.version,s.enabled,s.daily_request_limit,r.revision,r.provider_id,r.model_id,
 COALESCE(c.revoked_at IS NULL AND c.ciphertext IS NOT NULL,false),COALESCE(r.test_status,'untested'),r.last_tested_at,r.error_code
 FROM user_ai_settings s LEFT JOIN user_ai_config_revisions r ON r.id=s.current_revision_id AND r.owner_id=s.user_id
 LEFT JOIN ai_credentials c ON c.id=r.credential_id AND c.owner_id=s.user_id WHERE s.user_id=$1`, owner).Scan(&out.Version, &out.Enabled, &out.DailyRequestLimit, &out.Revision, &out.ProviderID, &out.ModelID, &out.HasKey, &out.TestStatus, &out.LastTestedAt, &out.ErrorCode)
	return out, err
}
func commitAI(ctx context.Context, tx *sql.Tx, owner string) (aisettings.Config, error) {
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s AISettingsStore) Get(ctx context.Context, owner string) (aisettings.Config, error) {
	tx, err := s.beginAI(ctx, owner)
	if err != nil {
		return aisettings.Config{}, err
	}
	defer tx.Rollback()
	return commitAI(ctx, tx, owner)
}
func (s AISettingsStore) Put(ctx context.Context, owner string, in aisettings.Put, vault aisettings.Vault) (aisettings.Config, error) {
	tx, err := s.beginAI(ctx, owner)
	if err != nil {
		return aisettings.Config{}, err
	}
	defer tx.Rollback()
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	if out.Version != in.ExpectedVersion {
		return out, aisettings.ErrConflict
	}
	changed := in.APIKey != nil || out.ModelID == nil || *out.ModelID != in.ModelID || out.ProviderID == nil || *out.ProviderID != in.ProviderID
	if in.APIKey == nil && (!out.HasKey || out.ProviderID == nil || *out.ProviderID != in.ProviderID) {
		return out, aisettings.ErrInvalid
	}
	if changed {
		var credentialID string
		if in.APIKey != nil {
			credentialID = security.UUID()
			plain := []byte(*in.APIKey)
			sealed, e := vault.Seal(owner, credentialID, plain)
			clear(plain)
			if e != nil {
				return out, aisettings.ErrUnavailable
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO ai_credentials(id,owner_id,ciphertext,nonce,key_version) VALUES($1,$2,$3,$4,$5)`, credentialID, owner, sealed.Ciphertext, sealed.Nonce, sealed.KeyVersion)
		} else {
			err = tx.QueryRowContext(ctx, `SELECT r.credential_id FROM user_ai_settings s JOIN user_ai_config_revisions r ON r.id=s.current_revision_id WHERE s.user_id=$1`, owner).Scan(&credentialID)
		}
		if err != nil {
			return out, err
		}
		revisionID := security.UUID()
		_, err = tx.ExecContext(ctx, `INSERT INTO user_ai_config_revisions(id,owner_id,revision,provider_id,model_id,credential_id) SELECT $2,user_id,revision_counter+1,$3,$4,$5 FROM user_ai_settings WHERE user_id=$1`, owner, revisionID, in.ProviderID, in.ModelID, credentialID)
		if err != nil {
			return out, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET current_revision_id=$2,revision_counter=revision_counter+1,enabled=false WHERE user_id=$1`, owner, revisionID)
		if err != nil {
			return out, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET version=version+1,daily_request_limit=$2,updated_at=now() WHERE user_id=$1`, owner, in.DailyRequestLimit)
	if err != nil {
		return out, err
	}
	return commitAI(ctx, tx, owner)
}
func (s AISettingsStore) Patch(ctx context.Context, owner string, in aisettings.Patch) (aisettings.Config, error) {
	tx, err := s.beginAI(ctx, owner)
	if err != nil {
		return aisettings.Config{}, err
	}
	defer tx.Rollback()
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	if out.Version != in.ExpectedVersion {
		return out, aisettings.ErrConflict
	}
	if in.Enabled != nil {
		if *in.Enabled && (!out.HasKey || out.TestStatus != "succeeded") {
			return out, aisettings.ErrTestRequired
		}
		out.Enabled = *in.Enabled
	}
	if in.DailyRequestLimit != nil {
		out.DailyRequestLimit = *in.DailyRequestLimit
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET version=version+1,enabled=$2,daily_request_limit=$3,updated_at=now() WHERE user_id=$1`, owner, out.Enabled, out.DailyRequestLimit)
	if err != nil {
		return out, err
	}
	return commitAI(ctx, tx, owner)
}
func (s AISettingsStore) Delete(ctx context.Context, owner string, version int64) (aisettings.Config, error) {
	if !aisettings.ValidVersion(version) {
		return aisettings.Config{}, aisettings.ErrInvalid
	}
	tx, err := s.beginAI(ctx, owner)
	if err != nil {
		return aisettings.Config{}, err
	}
	defer tx.Rollback()
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	if out.Version != version {
		return out, aisettings.ErrConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_credentials SET revoked_at=COALESCE(revoked_at,now()),ciphertext=NULL,nonce=NULL WHERE owner_id=$1`, owner)
	if err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_config_revisions SET revoked_at=COALESCE(revoked_at,now()),test_deadline=NULL,
 error_code=CASE WHEN test_status='testing' THEN 'test_interrupted' ELSE error_code END,
 test_status=CASE WHEN test_status='testing' THEN 'inconclusive' ELSE test_status END WHERE owner_id=$1`, owner)
	if err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET current_revision_id=NULL,enabled=false,version=version+1,updated_at=now() WHERE user_id=$1`, owner)
	if err != nil {
		return out, err
	}
	return commitAI(ctx, tx, owner)
}

// Expiration never retries the external call. A crash leaves a bounded,
// inconclusive result which can be polled/replayed safely.
func expireAITests(ctx context.Context, tx *sql.Tx, owner string) error {
	var run, revision string
	err := tx.QueryRowContext(ctx, `SELECT run_id,revision_id FROM ai_test_requests WHERE owner_id=$1 AND state='running' AND deadline<=clock_timestamp()`, owner).Scan(&run, &revision)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_config_revisions SET test_status='inconclusive',test_deadline=NULL,error_code='test_interrupted',last_tested_at=now() WHERE id=$1 AND owner_id=$2 AND test_run_id=$3 AND test_status='testing'`, revision, owner, run)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET version=version+1,enabled=false,updated_at=now() WHERE user_id=$1 AND current_revision_id=$2`, owner, revision)
	if err != nil {
		return err
	}
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return err
	}
	return saveAIReceipt(ctx, tx, owner, run, out)
}
func saveAIReceipt(ctx context.Context, tx *sql.Tx, owner, run string, out aisettings.Config) error {
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_test_requests SET state='done',safe_result=$3 WHERE owner_id=$1 AND run_id=$2 AND state='running'`, owner, run, raw)
	return err
}
