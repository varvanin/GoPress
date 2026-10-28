package backup

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxArchive int64 = 1 << 30

func WriteZIP(w io.Writer, dir string) error {
	all, err := files(dir)
	if err != nil {
		return err
	}
	all["manifest.json"] = ""
	archive := zip.NewWriter(w)
	for name := range all {
		in, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			archive.Close()
			return err
		}
		out, err := archive.Create(filepath.ToSlash(name))
		if err == nil {
			_, err = io.Copy(out, in)
		}
		err = errors.Join(err, in.Close())
		if err != nil {
			archive.Close()
			return err
		}
	}
	return archive.Close()
}
func ReadZIP(r io.ReaderAt, size int64, dest string) error {
	if size < 1 || size > MaxArchive {
		return errors.New("invalid backup archive size")
	}
	archive, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}
	if len(archive.File) > 100000 {
		return errors.New("too many backup files")
	}
	if err = os.Mkdir(dest, 0700); err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dest)
		}
	}()
	seen := map[string]bool{}
	var total uint64
	for _, file := range archive.File {
		name := file.Name
		if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") || filepath.ToSlash(filepath.Clean(name)) != strings.TrimSuffix(name, "/") || seen[name] || file.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid backup path")
		}
		seen[name] = true
		if file.FileInfo().IsDir() {
			continue
		}
		if !file.Mode().IsRegular() {
			return errors.New("special backup files are not allowed")
		}
		if name != "site.db" && name != "manifest.json" && !strings.HasPrefix(name, "uploads/") && !strings.HasPrefix(name, "theme/") {
			return errors.New("unexpected backup entry")
		}
		if file.UncompressedSize64 > uint64(MaxArchive) || total > uint64(MaxArchive)-file.UncompressedSize64 {
			return errors.New("backup archive too large")
		}
		total += file.UncompressedSize64
		path := filepath.Join(dest, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, MaxArchive+1))
		err = errors.Join(copyErr, out.Close(), in.Close())
		if err != nil {
			return err
		}
		if n > MaxArchive {
			return errors.New("backup file too large")
		}
	}
	if err = Verify(dest); err != nil {
		return err
	}
	ok = true
	return nil
}
