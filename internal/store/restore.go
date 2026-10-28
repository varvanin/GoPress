package store

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// ReplaceData keeps the process lock while swapping a verified staged site.
// The caller must drain all readers, writers and workers before calling it.
func (s *Store) ReplaceData(staged, rollback string) error {
	if filepath.Dir(staged) != s.Dir || filepath.Dir(rollback) != s.Dir {
		return errors.New("restore staging must be inside data directory")
	}
	if err := CheckFile(filepath.Join(staged, "site.db")); err != nil {
		return err
	}
	if err := os.Mkdir(rollback, 0700); err != nil {
		return err
	}
	if s.DB != nil {
		if err := s.DB.Close(); err != nil {
			return err
		}
	}
	names := []string{"site.db", "uploads", "theme"}
	old := []string{}
	incoming := []string{}
	var cause error
	for _, name := range names {
		err := os.Rename(filepath.Join(s.Dir, name), filepath.Join(rollback, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			cause = err
			break
		}
		old = append(old, name)
	}
	if cause == nil {
		for _, name := range names {
			err := os.Rename(filepath.Join(staged, name), filepath.Join(s.Dir, name))
			if errors.Is(err, os.ErrNotExist) && name != "site.db" {
				continue
			}
			if err != nil {
				cause = err
				break
			}
			incoming = append(incoming, name)
		}
	}
	if cause == nil {
		fresh, err := openWithLock(s.Dir, s.lock)
		if err == nil {
			*s = *fresh
			return nil
		}
		cause = err
	}
	// Reverse only completed moves; never remove the preserved original data.
	for i := len(incoming) - 1; i >= 0; i-- {
		name := incoming[i]
		if err := os.Rename(filepath.Join(s.Dir, name), filepath.Join(staged, name)); err != nil {
			return fmt.Errorf("restore failed (%v); quarantine failed: %w; original data: %s", cause, err, rollback)
		}
	}
	for i := len(old) - 1; i >= 0; i-- {
		name := old[i]
		if err := os.Rename(filepath.Join(rollback, name), filepath.Join(s.Dir, name)); err != nil {
			return fmt.Errorf("restore failed (%v); rollback failed: %w; original data: %s", cause, err, rollback)
		}
	}
	fresh, err := openWithLock(s.Dir, s.lock)
	if err != nil {
		return fmt.Errorf("restore failed (%v); reopening original data: %w", cause, err)
	}
	*s = *fresh
	return cause
}

// RecoverData is an offline repair path which does not open the damaged database.
func RecoverData(dir, staged, rollback string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(abs, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("stop the server before recovery: %w", err)
	}
	if err = os.WriteFile(filepath.Join(abs, "maintenance"), []byte("offline recovery\n"), 0600); err != nil {
		f.Close()
		return err
	}
	s := &Store{Dir: abs, lock: f}
	defer s.Close()
	if err = s.ReplaceData(staged, rollback); err != nil {
		return err
	}
	err = os.Remove(filepath.Join(abs, "restore-in-progress"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
