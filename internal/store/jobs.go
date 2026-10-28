package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (s *Store) ClaimFormEntry(ctx context.Context, now int64) (FormEntry, error) {
	// A crash during the final lease must leave a manually retryable failure.
	if _, err := s.DB.ExecContext(ctx, "UPDATE form_entries SET mail_status='failed',mail_claim_until=0 WHERE mail_status IN('pending','retry') AND mail_attempts>=5 AND mail_claim_until<=?", now); err != nil {
		return FormEntry{}, err
	}
	var e FormEntry
	var raw string
	err := s.DB.QueryRowContext(ctx, `UPDATE form_entries SET mail_claim_until=?,mail_attempts=mail_attempts+1 WHERE id=(SELECT id FROM form_entries WHERE mail_status IN('pending','retry') AND mail_next_at<=? AND mail_claim_until<=? AND mail_attempts<5 ORDER BY id LIMIT 1) RETURNING id,form_id,created_at,values_json,mail_status,mail_attempts`, now+60, now, now).Scan(&e.ID, &e.FormID, &e.CreatedAt, &raw, &e.MailStatus, &e.MailAttempts)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &e.Values)
	}
	return e, err
}
func (s *Store) FinishFormEntry(ctx context.Context, e FormEntry, now int64, delivered bool) error {
	status := "sent"
	next := int64(0)
	if !delivered {
		status = "retry"
		delays := []int64{15, 60, 300, 1800, 7200}
		index := e.MailAttempts - 1
		if index < 0 {
			index = 0
		}
		if index >= len(delays) {
			index = len(delays) - 1
		}
		next = now + delays[index]
		if e.MailAttempts >= 5 {
			status = "failed"
		}
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE form_entries SET mail_status=?,mail_next_at=?,mail_claim_until=0 WHERE id=? AND mail_attempts=? AND mail_claim_until>?", status, next, e.ID, e.MailAttempts, now-60)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) RetryFormEntry(ctx context.Context, formID, id, now int64) error {
	res, err := s.DB.ExecContext(ctx, "UPDATE form_entries SET mail_status='pending',mail_attempts=0,mail_next_at=0,mail_claim_until=0 WHERE id=? AND form_id=? AND mail_status!='sent' AND mail_claim_until<=?", id, formID, now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) DeleteFormEntries(ctx context.Context, formID int64, ids []int64, now int64) error {
	if len(ids) < 1 || len(ids) > 25 {
		return ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, "DELETE FROM form_entries WHERE id=? AND form_id=? AND mail_claim_until<=?", id, formID, now)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
	}
	if err = auditTx(ctx, tx, "forms.delete_entries", ""); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) PrunePrivateData(ctx context.Context, now int64, formDays, auditDays int) error {
	if formDays < 1 || auditDays < 1 {
		return ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM form_entries WHERE created_at<? AND mail_claim_until<=?", now-int64(formDays)*86400, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM audit_events WHERE at<?", now-int64(auditDays)*86400); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM password_resets WHERE expires_at<=?", now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) FormEntryBatch(formID, after, upper int64) ([]FormEntry, error) {
	rows, err := s.DB.Query("SELECT id,form_id,created_at,values_json,mail_status FROM form_entries WHERE form_id=? AND id>? AND id<=? ORDER BY id LIMIT 250", formID, after, upper)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []FormEntry{}
	for rows.Next() {
		var e FormEntry
		var raw string
		if err = rows.Scan(&e.ID, &e.FormID, &e.CreatedAt, &raw, &e.MailStatus); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &e.Values); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
func (s *Store) LastFormEntry(formID int64) (int64, error) {
	var id sql.NullInt64
	err := s.DB.QueryRow("SELECT max(id) FROM form_entries WHERE form_id=?", formID).Scan(&id)
	return id.Int64, err
}
