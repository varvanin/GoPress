package httpui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/store"
)

func TestWALRecoveryAfterProcessExit(t *testing.T) {
	if dir := os.Getenv("GOPRESS_WAL_HELPER"); dir != "" {
		s, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Install(context.Background(), store.Site{Title: "WAL test", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", []byte("unused test hash")); err != nil {
			t.Fatal(err)
		}
		p, err := s.Save(context.Background(), store.Page{Title: "Survives abrupt exit", Slug: "durable", Document: document}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Publish(context.Background(), p.ID, p.Version); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Deliberately bypass Close/checkpoint, like abrupt process termination.
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestWALRecoveryAfterProcessExit$")
	cmd.Env = append(os.Environ(), "GOPRESS_WAL_HELPER="+data)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v %s", err, out)
	}
	info, err := os.Stat(filepath.Join(data, "site.db-wal"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected nonempty WAL: %v", err)
	}
	snapshot := filepath.Join(root, "snapshot")
	if err = backup.Create(data, snapshot); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(root, "restored")
	if err = backup.Restore(snapshot, restored); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.Public("durable")
	if err != nil || p.Title != "Survives abrupt exit" {
		t.Fatalf("lost WAL content: %v", err)
	}
}
func TestBackupRejectsSymlinkAndMissingMedia(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	s, _ := openApp(t, data)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(data, filepath.Join(root, "linked-data")); err != nil {
		t.Fatal(err)
	}
	if err := backup.Create(data, filepath.Join(root, "linked-data", "snapshot")); err == nil {
		t.Fatal("backup accepted indirect destination inside data")
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(data, "uploads", "link")); err != nil {
		t.Fatal(err)
	}
	if err := backup.Create(data, filepath.Join(root, "bad-backup")); err == nil {
		t.Fatal("backup accepted symlink")
	}
	if err := os.Remove(filepath.Join(data, "uploads", "link")); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Media("missing.png", "image/png", 10); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = backup.Create(data, filepath.Join(root, "missing-media-backup")); err == nil {
		t.Fatal("backup accepted missing referenced upload")
	}
}
