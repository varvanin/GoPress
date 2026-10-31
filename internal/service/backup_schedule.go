package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"gopress/internal/store"
)

func (v *Service) SaveBackupSchedule(ctx context.Context, value store.BackupSchedule) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	if value.Version < 0 || value.Hour < 0 || value.Hour > 23 || value.Keep < 1 || value.Keep > 30 {
		return ErrInvalid
	}
	return v.Store.SaveBackupSchedule(ctx, value)
}
func backupDue(value store.BackupSchedule, now time.Time, zone *time.Location) bool {
	if !value.Enabled || now.In(zone).Hour() < value.Hour {
		return false
	}
	if value.Day != now.In(zone).Format("2006-01-02") {
		return true
	}
	return value.Failed && value.Attempts < 3 && now.Unix()-value.LastAttempt >= 3600
}
func (v *Service) RunScheduledBackup(ctx context.Context, now time.Time) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	v.Operations.RLock()
	value, err := v.Store.BackupSchedule()
	var site store.Site
	if err == nil && value.Enabled {
		site, err = v.Store.Site()
	}
	v.Operations.RUnlock()
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || !value.Enabled {
		return false, err
	}
	zone, err := time.LoadLocation(site.Timezone)
	if err != nil {
		return false, err
	}
	if !backupDue(value, now, zone) {
		return false, nil
	}
	v.Operations.Lock()
	defer v.Operations.Unlock()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	active, err := v.Maintenance()
	if err != nil || active {
		return false, err
	}
	value, err = v.Store.BackupSchedule()
	if err != nil {
		return false, err
	}
	site, err = v.Store.Site()
	if err != nil {
		return false, err
	}
	zone, err = time.LoadLocation(site.Timezone)
	if err != nil {
		return false, err
	}
	if !backupDue(value, now, zone) {
		return false, err
	}
	day := now.In(zone).Format("2006-01-02")
	if err = v.Store.ClaimScheduledBackup(ctx, day, now.Unix()); err != nil {
		return false, err
	}
	id, err := v.CreateBackup()
	if err != nil {
		return true, err
	}
	finish := context.WithoutCancel(ctx)
	if err = v.Store.FinishScheduledBackup(finish, id, now.Unix()); err != nil {
		return true, err
	}
	records, err := v.Store.ScheduledBackups()
	if err != nil {
		return true, err
	}
	for _, record := range records[min(value.Keep, len(records)):] {
		path, e := v.BackupPath(record.ID)
		if errors.Is(e, os.ErrNotExist) {
			e = nil
		} else if e == nil {
			e = os.RemoveAll(path)
		}
		if e != nil {
			return true, e
		}
		if e = v.Store.ForgetScheduledBackup(record.ID); e != nil {
			return true, e
		}
	}
	return true, v.Store.ClearBackupFailure(finish)
}
