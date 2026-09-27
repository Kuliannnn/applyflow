package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
)

func aiHash(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func (s AISettingsStore) StartTest(ctx context.Context, owner, key string, in aisettings.Test) (aisettings.Started, error) {
	tx, err := s.beginAI(ctx, owner)
	if err != nil {
		return aisettings.Started{}, err
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(in)
	requestHash := aiHash(raw)
	keyHash := aiHash([]byte(key))
	var storedHash, state string
	var result []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,state,safe_result FROM ai_test_requests WHERE owner_id=$1 AND key_hash=$2`, owner, keyHash).Scan(&storedHash, &state, &result)
	if err == nil {
		if storedHash != requestHash {
			return aisettings.Started{}, aisettings.ErrKeyConflict
		}
		var out aisettings.Config
		status := 200
		if state == "done" {
			err = json.Unmarshal(result, &out)
		} else {
			out, err = readAI(ctx, tx, owner)
			status = 202
		}
		if err != nil {
			return aisettings.Started{}, err
		}
		return aisettings.Started{Config: out, Status: status}, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return aisettings.Started{}, err
	}
	out, err := readAI(ctx, tx, owner)
	if err != nil {
		return aisettings.Started{}, err
	}
	if out.Version != in.ExpectedVersion {
		return aisettings.Started{}, aisettings.ErrConflict
	}
	if out.Revision == nil || *out.Revision != in.Revision || !out.HasKey {
		return aisettings.Started{}, aisettings.ErrChanged
	}
	var running bool
	var minute, day int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_test_requests WHERE owner_id=$1 AND state='running'),count(*) FILTER(WHERE created_at>now()-interval '1 minute'),count(*) FROM ai_test_requests WHERE owner_id=$1 AND created_at>now()-interval '1 day'`, owner).Scan(&running, &minute, &day)
	if err != nil {
		return aisettings.Started{}, err
	}
	if running {
		return aisettings.Started{}, aisettings.ErrBusy
	}
	if minute >= 3 || day >= 20 {
		return aisettings.Started{}, aisettings.ErrRateLimited
	}
	claim := aisettings.Claim{OwnerID: owner, RunID: security.UUID()}
	err = tx.QueryRowContext(ctx, `SELECT r.id,r.credential_id,r.model_id,c.ciphertext,c.nonce,c.key_version FROM user_ai_settings s JOIN user_ai_config_revisions r ON r.id=s.current_revision_id AND r.owner_id=s.user_id JOIN ai_credentials c ON c.id=r.credential_id AND c.owner_id=r.owner_id WHERE s.user_id=$1 AND r.revoked_at IS NULL AND c.revoked_at IS NULL`, owner).Scan(&claim.RevisionID, &claim.CredentialID, &claim.ModelID, &claim.Sealed.Ciphertext, &claim.Sealed.Nonce, &claim.Sealed.KeyVersion)
	if err != nil {
		return aisettings.Started{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_test_requests(owner_id,key_hash,request_hash,revision_id,run_id,state,deadline) VALUES($1,$2,$3,$4,$5,'running',clock_timestamp()+interval '12 seconds')`, owner, keyHash, requestHash, claim.RevisionID, claim.RunID)
	if err != nil {
		return aisettings.Started{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_config_revisions SET test_status='testing',test_run_id=$2,test_deadline=(SELECT deadline FROM ai_test_requests WHERE run_id=$2),error_code=NULL WHERE id=$1`, claim.RevisionID, claim.RunID)
	if err != nil {
		return aisettings.Started{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET enabled=false,version=version+1,updated_at=now() WHERE user_id=$1`, owner)
	if err != nil {
		return aisettings.Started{}, err
	}
	out, err = readAI(ctx, tx, owner)
	if err != nil {
		return aisettings.Started{}, err
	}
	return aisettings.Started{Config: out, Status: 202, Claim: &claim}, tx.Commit()
}
func (s AISettingsStore) FinishTest(ctx context.Context, claim aisettings.Claim, result aisettings.ProbeResult) (aisettings.Config, error) {
	tx, err := s.beginAI(ctx, claim.OwnerID)
	if err != nil {
		return aisettings.Config{}, err
	}
	defer tx.Rollback()
	var state string
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT state,safe_result FROM ai_test_requests WHERE owner_id=$1 AND run_id=$2 AND revision_id=$3`, claim.OwnerID, claim.RunID, claim.RevisionID).Scan(&state, &raw)
	if err != nil {
		return aisettings.Config{}, err
	}
	if state == "done" {
		var out aisettings.Config
		if err = json.Unmarshal(raw, &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	// Match both immutable revision and run. A deleted/replaced configuration
	// cannot be made ready by a late result, even if the provider succeeded.
	updated, err := tx.ExecContext(ctx, `UPDATE user_ai_config_revisions r SET test_status=$4,error_code=NULLIF($5,''),test_deadline=NULL,last_tested_at=now()
 WHERE r.owner_id=$1 AND r.id=$2 AND r.test_run_id=$3 AND r.test_status='testing' AND r.revoked_at IS NULL
 AND EXISTS(SELECT 1 FROM user_ai_settings s WHERE s.user_id=$1 AND s.current_revision_id=r.id)`, claim.OwnerID, claim.RevisionID, claim.RunID, result.Status, result.Code)
	if err != nil {
		return aisettings.Config{}, err
	}
	n, err := updated.RowsAffected()
	if err != nil {
		return aisettings.Config{}, err
	}
	if n > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE user_ai_settings SET version=version+1,enabled=false,updated_at=now() WHERE user_id=$1`, claim.OwnerID)
		if err != nil {
			return aisettings.Config{}, err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE user_ai_config_revisions SET test_status='inconclusive',test_deadline=NULL,error_code='test_interrupted',last_tested_at=now() WHERE id=$1 AND owner_id=$2 AND test_run_id=$3 AND test_status='testing'`, claim.RevisionID, claim.OwnerID, claim.RunID)
		if err != nil {
			return aisettings.Config{}, err
		}
	}
	out, err := readAI(ctx, tx, claim.OwnerID)
	if err != nil {
		return out, err
	}
	if err = saveAIReceipt(ctx, tx, claim.OwnerID, claim.RunID, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
