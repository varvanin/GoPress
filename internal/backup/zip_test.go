package backup_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/store"
)

func TestZIPBackupRoundTripAndTraversalRejection(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "site")
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), store.Site{Title: "ZIP", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "backup")
	if err = backup.CreateFromStore(s, source); err != nil {
		t.Fatal(err)
	}
	s.Close()
	var archive bytes.Buffer
	if err = backup.WriteZIP(&archive, source); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "imported")
	if err = backup.ReadZIP(bytes.NewReader(archive.Bytes()), int64(archive.Len()), dest); err != nil {
		t.Fatal(err)
	}
	if err = backup.Verify(dest); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"../escape", "uploads/../../escape", "/absolute", "mail.json"} {
		var bad bytes.Buffer
		writer := zip.NewWriter(&bad)
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte("bad"))
		writer.Close()
		if err = backup.ReadZIP(bytes.NewReader(bad.Bytes()), int64(bad.Len()), filepath.Join(root, "bad-"+string(rune('a'+i)))); err == nil {
			t.Fatal("unsafe entry accepted", name)
		}
	}
	if _, err = os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatal("archive escaped destination", err)
	}
}

func TestRecoverInterruptedRestorePreservesDamagedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "site")
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), store.Site{Title: "Recovered", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "backup")
	if err = backup.CreateFromStore(s, source); err != nil {
		t.Fatal(err)
	}
	if err = backup.Recover(source, dir); err == nil {
		t.Fatal("recovered while server owns lock")
	}
	s.Close()
	if err = os.WriteFile(filepath.Join(dir, "site.db"), []byte("broken database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "restore-in-progress"), []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if opened, err := store.Open(dir); err == nil {
		opened.Close()
		t.Fatal("opened interrupted restore")
	}
	if err = backup.Recover(source, dir); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	site, err := s.Site()
	if err != nil || site.Title != "Recovered" {
		t.Fatal(site, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "maintenance")); err != nil {
		t.Fatal(err)
	}
	previous, err := filepath.Glob(filepath.Join(dir, ".recovery-old-*", "site.db"))
	if err != nil || len(previous) != 1 {
		t.Fatal(previous, err)
	}
	damaged, err := os.ReadFile(previous[0])
	if err != nil || string(damaged) != "broken database" {
		t.Fatal(string(damaged), err)
	}
}
