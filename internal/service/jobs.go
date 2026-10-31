package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

func (v *Service) ProcessFormMail(ctx context.Context, now int64) (bool, error) {
	settings, err := v.MailSettings()
	if err != nil {
		return false, err
	}
	if !settings.Configured() {
		return false, nil
	}
	entry, err := v.Store.ClaimFormEntry(ctx, now)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	form, err := v.Store.Form(entry.FormID)
	if err == nil {
		err = v.sendFormEntry(ctx, settings, form, entry)
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if finishErr := v.Store.FinishFormEntry(finishCtx, entry, time.Now().Unix(), err == nil); finishErr != nil {
		return true, finishErr
	}
	return true, nil
}
func (v *Service) RunMailWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var cleaned int64
	for {
		if _, err := v.RunScheduledBackup(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("scheduled backup failed", "error", err)
		}
		for i := 0; i < 10; i++ {
			v.Operations.RLock()
			active, err := v.Maintenance()
			worked := false
			now := time.Now().Unix()
			if err == nil && !active {
				if now-cleaned >= 3600 {
					settings, e := v.Store.Settings()
					if e == nil {
						e = v.Store.PrunePrivateData(ctx, now, settings.FormRetentionDays, settings.AuditRetentionDays)
					}
					if errors.Is(e, sql.ErrNoRows) {
						e = nil
					}
					if e != nil {
						err = e
					} else {
						cleaned = now
					}
				}
				if err == nil {
					worked, err = v.ProcessFormMail(ctx, now)
				}
			}
			v.Operations.RUnlock()
			if err != nil && ctx.Err() == nil {
				slog.Error("form mail worker failed", "error", err)
			}
			if !worked || err != nil || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
