// Package backup implements offline directory snapshots, never HTTP downloads.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopress/internal/store"
)

type Manifest struct {
	Format int               `json:"format"`
	Files  map[string]string `json:"sha256"`
}

func outside(src, dst string) error {
	a, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	a, err = filepath.EvalSymlinks(a)
	if err != nil {
		return err
	}
	b, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(b))
	if err != nil {
		return err
	}
	b = filepath.Join(parent, filepath.Base(b))
	r, err := filepath.Rel(a, b)
	if err != nil {
		return err
	}
	if r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return errors.New("backup destination must be outside the source directory")
	}
	return nil
}
func stage(dest string) (string, error) {
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("destination must not exist")
	}
	return os.MkdirTemp(filepath.Dir(dest), ".gopress-stage-")
}
func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("non-regular backup file")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}
func files(dir string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlinks are not allowed in backups")
		}
		if d.IsDir() {
			return nil
		}
		if rel == "manifest.json" {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("non-regular file")
		}
		if rel != "site.db" && !strings.HasPrefix(rel, "uploads/") && !strings.HasPrefix(rel, "theme/") {
			return fmt.Errorf("unexpected backup file: %s", rel)
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		e = errors.Join(e, f.Close())
		if e != nil {
			return e
		}
		result[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return result, err
}
func Verify(dir string) error {
	f, err := os.Open(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	var m Manifest
	d := json.NewDecoder(io.LimitReader(f, 4<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid manifest")
	}
	if m.Format != 1 || m.Files["site.db"] == "" {
		return errors.New("unsupported backup format")
	}
	actual, err := files(dir)
	if err != nil {
		return err
	}
	if len(actual) != len(m.Files) {
		return errors.New("backup file count mismatch")
	}
	for p, h := range m.Files {
		if actual[p] != h {
			return fmt.Errorf("backup checksum mismatch: %s", p)
		}
	}
	return store.CheckFile(filepath.Join(dir, "site.db"))
}
func Create(data, dest string) error {
	if err := outside(data, dest); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(data, "site.db")); err != nil {
		return err
	}
	s, err := store.Open(data)
	if err != nil {
		return err
	}
	defer s.Close()
	return CreateFromStore(s, dest)
}

// CreateFromStore requires the caller to exclude all writes until it returns.
func CreateFromStore(s *store.Store, dest string) error {
	tmp, err := stage(dest)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err = s.Snapshot(filepath.Join(tmp, "site.db")); err != nil {
		return err
	}
	for _, sub := range []string{"uploads", "theme"} {
		root := filepath.Join(s.Dir, sub)
		if _, e := os.Lstat(root); errors.Is(e, os.ErrNotExist) {
			continue
		}
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("symlink in data directory")
			}
			if d.IsDir() {
				return nil
			}
			rel, e := filepath.Rel(s.Dir, p)
			if e != nil {
				return e
			}
			return copyFile(p, filepath.Join(tmp, rel))
		})
		if err != nil {
			return err
		}
	}
	all, err := files(tmp)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(Manifest{Format: 1, Files: all}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(tmp, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	if err = Verify(tmp); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
func Restore(src, dest string) error {
	if err := outside(src, dest); err != nil {
		return err
	}
	if err := Verify(src); err != nil {
		return err
	}
	tmp, err := stage(dest)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	all, err := files(src)
	if err != nil {
		return err
	}
	for p := range all {
		if err = copyFile(filepath.Join(src, p), filepath.Join(tmp, p)); err != nil {
			return err
		}
	}
	// Re-check the staged copy so a changed source cannot silently corrupt a restore.
	raw, err := os.ReadFile(filepath.Join(src, "manifest.json"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(tmp, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	if err = Verify(tmp); err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(tmp, "manifest.json")); err != nil {
		return err
	}
	s, err := store.Open(tmp)
	if err != nil {
		return err
	}
	err = s.RevokeSessions()
	err = errors.Join(err, s.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// Recover replaces core files in an existing, stopped site, preserving the previous files.
func Recover(src, data string) error {
	if err := Verify(src); err != nil {
		return err
	}
	abs, err := filepath.Abs(data)
	if err != nil {
		return err
	}
	if _, err = os.Stat(abs); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(abs, ".recover-stage-")
	if err != nil {
		return err
	}
	if err = os.Remove(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = Restore(src, stage); err != nil {
		return err
	}
	rollback, err := os.MkdirTemp(abs, ".recovery-old-")
	if err != nil {
		return err
	}
	if err = os.Remove(rollback); err != nil {
		return err
	}
	return store.RecoverData(abs, stage, rollback)
}
