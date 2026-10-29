package service

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/store"
)

type importProbe struct {
	io.ReaderAt
	check func()
}

func (r importProbe) ReadAt(p []byte, off int64) (int, error) {
	r.check()
	return r.ReaderAt.ReadAt(p, off)
}

func TestImportedBackupAppearsOnlyAfterVerification(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Install(context.Background(), store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err = backup.CreateFromStore(s, snapshot); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err = backup.WriteZIP(&archive, snapshot); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(s.Dir, "backups")
	reader := importProbe{ReaderAt: bytes.NewReader(archive.Bytes()), check: func() {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if backupID.MatchString(entry.Name()) {
				t.Error("an unfinished import is visible as a backup")
			}
		}
	}}
	v := &Service{Store: s}
	id, err := v.ImportBackup(reader, int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	backups, err := v.Backups()
	if err != nil || len(backups) != 1 || backups[0].ID != id {
		t.Fatal(backups, err)
	}
	if err = backup.Verify(filepath.Join(root, id)); err != nil {
		t.Fatal(err)
	}
}
