package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sys/unix"
	"gopress/internal/backup"
	"gopress/internal/store"
)

var backupID = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}-[a-f0-9]{8}$`)

type BackupInfo struct {
	ID    string
	At    int64
	Bytes int64
}
type Diagnostics struct {
	Schema                int
	FreeBytes             uint64
	DatabaseBytes         int64
	Users, Pages, Entries int
}

func (v *Service) Maintenance() (bool, error) {
	_, err := os.Stat(filepath.Join(v.Store.Dir, "maintenance"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
func (v *Service) SetMaintenance(enabled bool) error {
	path := filepath.Join(v.Store.Dir, "maintenance")
	if enabled {
		return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0600)
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (v *Service) Backups() ([]BackupInfo, error) {
	root := filepath.Join(v.Store.Dir, "backups")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []BackupInfo{}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() || !backupID.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		var size int64
		if err = filepath.WalkDir(filepath.Join(root, entry.Name()), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				size += info.Size()
			}
			return nil
		}); err != nil {
			return nil, err
		}
		result = append(result, BackupInfo{ID: entry.Name(), At: info.ModTime().Unix(), Bytes: size})
		if len(result) == 100 {
			break
		}
	}
	return result, nil
}
func (v *Service) BackupPath(id string) (string, error) {
	if !backupID.MatchString(id) {
		return "", ErrInvalid
	}
	path := filepath.Join(v.Store.Dir, "backups", id)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalid
	}
	return path, nil
}
func (v *Service) CreateBackup() (string, error) {
	root := filepath.Join(v.Store.Dir, "backups")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + Token()[:8]
	if err := backup.CreateFromStore(v.Store, filepath.Join(root, id)); err != nil {
		return "", err
	}
	return id, nil
}
func (v *Service) RestoreBackup(id string) error {
	active, err := v.Maintenance()
	if err != nil {
		return err
	}
	if !active {
		return errors.New("maintenance mode is required")
	}
	source, err := v.BackupPath(id)
	if err != nil {
		return err
	}
	if err = backup.Verify(source); err != nil {
		return err
	}
	if _, err = v.CreateBackup(); err != nil {
		return err
	}
	stage := filepath.Join(v.Store.Dir, ".restore-"+Token()[:16])
	defer os.RemoveAll(stage)
	if err = backup.Restore(source, stage); err != nil {
		return err
	}
	rollback := filepath.Join(v.Store.Dir, ".rollback-"+Token()[:16])
	marker := filepath.Join(v.Store.Dir, "restore-in-progress")
	if err = os.WriteFile(marker, []byte("backup="+id+"\nrollback="+rollback+"\n"), 0600); err != nil {
		return err
	}
	if err = v.Store.ReplaceData(stage, rollback); err != nil {
		if v.Store.DB != nil && v.Store.DB.Ping() == nil && store.CheckFile(filepath.Join(v.Store.Dir, "site.db")) == nil {
			os.Remove(marker)
		}
		return err
	}
	if err = os.Remove(marker); err != nil {
		return err
	}
	// A verified normal backup already preserves the previous site.
	return os.RemoveAll(rollback)
}
func (v *Service) Diagnose() (Diagnostics, error) {
	var d Diagnostics
	if err := v.Store.Check(); err != nil {
		return d, err
	}
	if err := v.Store.DB.QueryRow("PRAGMA user_version").Scan(&d.Schema); err != nil {
		return d, err
	}
	for query, target := range map[string]*int{"SELECT count(*) FROM users": &d.Users, "SELECT count(*) FROM pages": &d.Pages, "SELECT count(*) FROM form_entries": &d.Entries} {
		if err := v.Store.DB.QueryRow(query).Scan(target); err != nil {
			return d, err
		}
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(v.Store.Dir, &stat); err != nil {
		return d, err
	}
	d.FreeBytes = uint64(stat.Bavail) * uint64(stat.Bsize)
	info, err := os.Stat(filepath.Join(v.Store.Dir, "site.db"))
	if err != nil {
		return d, err
	}
	d.DatabaseBytes = info.Size()
	return d, nil
}
func (v *Service) AdminResetPassword(ctx context.Context, email, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return ErrInvalid
	}
	u, err := v.Store.UserByEmail(email)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if err = v.Store.ChangePassword(ctx, u.ID, nil, hash); err != nil {
		return err
	}
	return v.Store.Audit(ctx, "cli.reset_password", strconv.FormatInt(u.ID, 10))
}

func (v *Service) ImportBackup(r io.ReaderAt, size int64) (string, error) {
	root := filepath.Join(v.Store.Dir, "backups")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + Token()[:8]
	stage := filepath.Join(root, ".import-"+Token()[:16])
	defer os.RemoveAll(stage)
	if err := backup.ReadZIP(r, size, stage); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := os.Rename(stage, filepath.Join(root, id)); err != nil {
		return "", err
	}
	return id, nil
}
