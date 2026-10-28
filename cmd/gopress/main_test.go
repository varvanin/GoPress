package main

import (
	"context"
	"path/filepath"
	"testing"

	"gopress/internal/store"
)

func TestBackupCLI(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "site")
	s, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), store.Site{Title: "CLI site", BaseURL: "http://localhost:8080", Timezone: "UTC"}, "cli@example.com", []byte("unused test hash")); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "backup")
	restore := filepath.Join(root, "restored")
	for _, args := range [][]string{{"version"}, {"backup", "create", "--data", data, "--output", snapshot}, {"backup", "verify", "--source", snapshot}, {"backup", "restore", "--source", snapshot, "--data", restore}} {
		if err = run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	s, err = store.Open(restore)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	site, err := s.Site()
	if err != nil || site.Title != "CLI site" {
		t.Fatalf("restored site: %+v %v", site, err)
	}
	for _, args := range [][]string{nil, {"unknown"}, {"backup"}, {"backup", "unknown"}, {"backup", "create"}, {"backup", "verify"}, {"backup", "restore"}, {"backup", "verify", "--source", snapshot, "unexpected"}} {
		if err = run(args); err == nil {
			t.Fatalf("accepted invalid args: %v", args)
		}
	}
}

func TestOperationalCommandsRequireExistingDatabase(t *testing.T) {
	for _, command := range []string{"doctor", "migrate", "search", "admin"} {
		dir := filepath.Join(t.TempDir(), "missing")
		args := []string{command}
		if command == "search" {
			args = append(args, "rebuild")
		}
		if command == "admin" {
			args = append(args, "reset-password")
		}
		args = append(args, "--data", dir)
		if err := run(args); err == nil {
			t.Fatal("created database for", command)
		}
	}
}
