package store

import (
	"context"
	"database/sql"
	"strconv"
)

type SecurityProfile struct {
	UserID, Version, LastCounter int64
	Identity                     string
	Enabled                      bool
	RecoveryCount                int
}
type SecondFactorProof struct {
	Counter      *int64
	RecoveryHash string
}
type SessionAuth struct {
	Profile   SecurityProfile
	Proof     SecondFactorProof
	UserAgent string
}
type SessionInfo struct {
	Reference, UserAgent string
	CreatedAt, ExpiresAt int64
	Current              bool
}

func (s *Store) SecurityProfile(id int64) (SecurityProfile, error) {
	var v SecurityProfile
	v.UserID = id
	err := s.DB.QueryRow(`SELECT identity,version,enabled,last_counter,(SELECT count(*) FROM user_recovery_codes WHERE user_id=?) FROM user_security WHERE user_id=?`, id, id).Scan(&v.Identity, &v.Version, &v.Enabled, &v.LastCounter, &v.RecoveryCount)
	return v, err
}
func consumeSecondFactor(tx *sql.Tx, profile SecurityProfile, proof SecondFactorProof) error {
	if !profile.Enabled {
		return nil
	}
	var result sql.Result
	var err error
	if proof.Counter != nil && proof.RecoveryHash == "" && *proof.Counter >= 0 {
		result, err = tx.Exec(`UPDATE user_security SET last_counter=? WHERE user_id=? AND identity=? AND version=? AND enabled=1 AND last_counter<?`, *proof.Counter, profile.UserID, profile.Identity, profile.Version, *proof.Counter)
	} else if proof.Counter == nil && len(proof.RecoveryHash) == 64 {
		result, err = tx.Exec(`DELETE FROM user_recovery_codes WHERE user_id=? AND hash=? AND EXISTS(SELECT 1 FROM user_security WHERE user_id=? AND identity=? AND version=? AND enabled=1)`, profile.UserID, proof.RecoveryHash, profile.UserID, profile.Identity, profile.Version)
	} else {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) ConfigureSecondFactor(ctx context.Context, user User, passwordHash []byte, profile SecurityProfile, enabled bool, hashes []string, proof SecondFactorProof) error {
	if profile.UserID != user.ID || !profile.Enabled && enabled && (proof.Counter == nil || *proof.Counter < 0) {
		return ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE users SET version=version+1 WHERE id=? AND version=? AND password_hash=? AND blocked=0`, user.ID, user.Version, passwordHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if err = consumeSecondFactor(tx, profile, proof); err != nil {
		return err
	}
	counter := int64(-1)
	if proof.Counter != nil {
		counter = *proof.Counter
	}
	result, err = tx.ExecContext(ctx, `UPDATE user_security SET version=version+1,enabled=?,last_counter=CASE WHEN ?=0 THEN -1 WHEN enabled=0 THEN ? ELSE last_counter END WHERE user_id=? AND identity=? AND version=? AND enabled=?`, enabled, enabled, counter, user.ID, profile.Identity, profile.Version, profile.Enabled)
	if err != nil {
		return err
	}
	n, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id=?`, user.ID); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO user_recovery_codes(user_id,hash) VALUES(?,?)`, user.ID, hash); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, user.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id=?`, user.ID); err != nil {
		return err
	}
	action := "user.2fa_disabled"
	if enabled {
		action = "user.2fa_enabled"
		if profile.Enabled {
			action = "user.recovery_codes_rotated"
		}
	}
	if err = auditTx(ctx, tx, action, strconv.FormatInt(user.ID, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

// This recovery path is called only by the offline CLI holding the site's directory lock.
func (s *Store) ResetSecondFactorOffline(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE user_security SET enabled=0,version=version+1,last_counter=-1 WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE users SET version=version+1 WHERE id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM user_recovery_codes WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM password_resets WHERE user_id=?`, id); err != nil {
		return err
	}
	if err = auditTx(context.Background(), tx, "user.2fa_reset", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) UserSessions(id int64, current string) ([]SessionInfo, int, error) {
	var total int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sessions WHERE user_id=? AND expires_at>unixepoch()`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(`SELECT s.hash,s.expires_at,coalesce(d.created_at,0),coalesce(d.user_agent,'') FROM sessions s LEFT JOIN session_details d ON d.hash=s.hash WHERE s.user_id=? AND s.expires_at>unixepoch() ORDER BY (s.hash=?) DESC,coalesce(d.created_at,0) DESC,s.hash LIMIT 50`, id, current)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []SessionInfo{}
	for rows.Next() {
		var v SessionInfo
		if err = rows.Scan(&v.Reference, &v.ExpiresAt, &v.CreatedAt, &v.UserAgent); err != nil {
			return nil, 0, err
		}
		v.Current = v.Reference == current
		result = append(result, v)
	}
	return result, total, rows.Err()
}
func (s *Store) RevokeUserSession(ctx context.Context, id int64, current, reference string) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND hash=? AND EXISTS(SELECT 1 FROM sessions current WHERE current.user_id=? AND current.hash=? AND current.expires_at>unixepoch())`, id, reference, id, current)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}
func (s *Store) RevokeOtherSessions(ctx context.Context, id int64, current string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND hash<>? AND EXISTS(SELECT 1 FROM sessions current WHERE current.user_id=? AND current.hash=? AND current.expires_at>unixepoch())`, id, current, id, current)
	return err
}
