package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopress/internal/backup"
	"gopress/internal/store"
)

func scheduledBackupService(t *testing.T) *Service {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Install(context.Background(), v.InstallToken, store.Site{Title: "Backup test", BaseURL: "https://example.test", Timezone: "Europe/Warsaw"}, "owner@example.test", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	value, err := s.BackupSchedule()
	if err != nil {
		t.Fatal(err)
	}
	value.Enabled = true
	value.Hour = 3
	value.Keep = 2
	if err = v.SaveBackupSchedule(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestScheduledBackupsUseSiteTimeAndRetainOnlyAutomaticCopies(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	manual, err := v.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2027, 1, 15, 1, 0, 0, 0, time.UTC)
	if worked, err := v.RunScheduledBackup(ctx, before); worked || err != nil {
		t.Fatal("ran before configured local hour", worked, err)
	}
	now := before.Add(time.Hour)
	for day := 0; day < 3; day++ {
		when := now.AddDate(0, 0, day)
		if worked, err := v.RunScheduledBackup(ctx, when); !worked || err != nil {
			t.Fatal("missing scheduled backup", day, worked, err)
		}
		if worked, err := v.RunScheduledBackup(ctx, when.Add(time.Minute)); worked || err != nil {
			t.Fatal("duplicated daily backup", worked, err)
		}
	}
	if _, err = v.BackupPath(manual); err != nil {
		t.Fatal("retention removed manual backup", err)
	}
	records, err := v.Store.ScheduledBackups()
	if err != nil || len(records) != 2 {
		t.Fatal(records, err)
	}
	files, err := v.Backups()
	if err != nil || len(files) != 3 {
		t.Fatal(files, err)
	}
	automatic := 0
	for _, file := range files {
		if file.Automatic {
			automatic++
		}
	}
	if automatic != 2 {
		t.Fatal("wrong automatic markers", files)
	}
	for _, record := range records {
		path, e := v.BackupPath(record.ID)
		if e != nil {
			t.Fatal(e)
		}
		if e = backup.Verify(path); e != nil {
			t.Fatal("invalid scheduled copy", e)
		}
	}
	state, err := v.Store.BackupSchedule()
	if err != nil || state.Failed || state.LastSuccess != now.AddDate(0, 0, 2).Unix() {
		t.Fatal(state, err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	source, _ := v.BackupPath(state.LastID)
	if err = backup.Restore(source, restored); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restoredState, err := s.BackupSchedule()
	if err != nil || !restoredState.Enabled || restoredState.LastAttempt != 0 || restoredState.Failed {
		t.Fatal("restored transient job state", restoredState, err)
	}
}
func TestScheduledBackupFailuresRetryHourlyAtMostThreeTimesPerDay(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(v.Store.Dir, "backups"), []byte("blocked directory"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 1, 15, 2, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{0, time.Hour, 2 * time.Hour} {
		if worked, err := v.RunScheduledBackup(ctx, now.Add(offset)); !worked || err == nil {
			t.Fatal("failure not reported", offset, worked, err)
		}
	}
	if worked, err := v.RunScheduledBackup(ctx, now.Add(3*time.Hour)); worked || err != nil {
		t.Fatal("retry budget exceeded", worked, err)
	}
	state, err := v.Store.BackupSchedule()
	if err != nil || !state.Failed || state.Attempts != 3 {
		t.Fatal(state, err)
	}
	if worked, err := v.RunScheduledBackup(ctx, now.AddDate(0, 0, 1)); !worked || err == nil {
		t.Fatal("daily retry budget did not reset", worked, err)
	}
}
func TestScheduledBackupSerializesConcurrentClaimsAndHonorsMaintenance(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	now := time.Date(2027, 1, 15, 2, 0, 0, 0, time.UTC)
	if err := v.SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	if worked, err := v.RunScheduledBackup(ctx, now); worked || err != nil {
		t.Fatal(worked, err)
	}
	_ = v.SetMaintenance(false)
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worked, err := v.RunScheduledBackup(ctx, now)
			results <- worked
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	count := 0
	for worked := range results {
		if worked {
			count++
		}
	}
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatal("more than one daily claim", count)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := v.RunScheduledBackup(canceled, now); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	value, _ := v.Store.BackupSchedule()
	value.Hour = 24
	if err := v.SaveBackupSchedule(ctx, value); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid schedule accepted", err)
	}
}

func TestDailyBackupUsesCalendarDaysAcrossDST(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	value := store.BackupSchedule{Enabled: true, Hour: 2}
	spring := time.Date(2027, 3, 28, 1, 0, 0, 0, time.UTC)
	if !backupDue(value, spring, zone) {
		t.Fatal("skipped nonexistent spring hour")
	}
	value.Day = "2027-10-31"
	value.LastAttempt = time.Date(2027, 10, 31, 0, 30, 0, 0, time.UTC).Unix()
	value.LastSuccess = value.LastAttempt
	value.Attempts = 1
	if backupDue(value, time.Date(2027, 10, 31, 1, 30, 0, 0, time.UTC), zone) {
		t.Fatal("ran twice during repeated autumn hour")
	}
}
