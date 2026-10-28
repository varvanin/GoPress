package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/service"
	"gopress/internal/store"
)

func TestBackupExcludesMailSecrets(t *testing.T) {
	t.Setenv("GOPRESS_SMTP_HOST", "")
	t.Setenv("GOPRESS_SMTP_FROM", "")
	root := t.TempDir()
	dir := filepath.Join(root, "site")
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := service.New(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Install(context.Background(), v.InstallToken, store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err = v.SaveMailSettings(service.MailSettings{Enabled: true, Host: "smtp.example.com", Port: "587", From: "sender@example.com", Password: "PRIVATE-SMTP-SECRET"}, false); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "backup")
	if err = backup.Create(dir, dest); err != nil {
		t.Fatal(err)
	}
	if err = backup.Verify(dest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, "mail.json")); !os.IsNotExist(err) {
		t.Fatal("SMTP file included in backup", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dest, "manifest.json"))
	if err != nil || strings.Contains(string(manifest), "mail.json") {
		t.Fatal("mail file in manifest", err)
	}
	restored := filepath.Join(root, "restored")
	if err = backup.Restore(dest, restored); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings, err := s.Settings()
	if err != nil || settings.Site.Title != "Site" {
		t.Fatal("settings not restored", err)
	}
}
