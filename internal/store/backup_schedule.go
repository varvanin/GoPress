package store

import "context"

type BackupSchedule struct {
	Version                  int64
	Enabled                  bool
	Hour, Keep, Attempts     int
	Day                      string
	LastAttempt, LastSuccess int64
	LastID                   string
	Failed                   bool
}
type ScheduledBackup struct {
	ID string
	At int64
}

func (s *Store) BackupSchedule() (BackupSchedule, error) {
	var v BackupSchedule
	err := s.DB.QueryRow(`SELECT version,enabled,hour,keep,attempts,day,last_attempt,last_success,last_id,failed FROM backup_schedule WHERE id=1`).Scan(&v.Version, &v.Enabled, &v.Hour, &v.Keep, &v.Attempts, &v.Day, &v.LastAttempt, &v.LastSuccess, &v.LastID, &v.Failed)
	return v, err
}
func (s *Store) SaveBackupSchedule(ctx context.Context, v BackupSchedule) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE backup_schedule SET version=version+1,enabled=?,hour=?,keep=? WHERE id=1 AND version=?`, v.Enabled, v.Hour, v.Keep, v.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) ClaimScheduledBackup(ctx context.Context, day string, now int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE backup_schedule SET attempts=CASE WHEN day=? THEN attempts+1 ELSE 1 END,day=?,last_attempt=?,failed=1 WHERE id=1`, day, day, now)
	return err
}
func (s *Store) FinishScheduledBackup(ctx context.Context, id string, now int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO scheduled_backups(id,created_at) VALUES(?,?)`, id, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE backup_schedule SET last_success=?,last_id=? WHERE id=1`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ClearBackupFailure(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE backup_schedule SET failed=0 WHERE id=1`)
	return err
}
func (s *Store) ScheduledBackups() ([]ScheduledBackup, error) {
	rows, err := s.DB.Query(`SELECT id,created_at FROM scheduled_backups ORDER BY created_at DESC,id DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ScheduledBackup{}
	for rows.Next() {
		var v ScheduledBackup
		if err = rows.Scan(&v.ID, &v.At); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) ForgetScheduledBackup(id string) error {
	_, err := s.DB.Exec(`DELETE FROM scheduled_backups WHERE id=?`, id)
	return err
}

func (s *Store) ResetBackupScheduleState() error {
	_, err := s.DB.Exec(`UPDATE backup_schedule SET attempts=0,day='',last_attempt=0,last_success=0,last_id='',failed=0 WHERE id=1`)
	return err
}
