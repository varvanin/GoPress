package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSessionPruningKeepsLiveSessionsAndLookups(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveUser(ctx, User{Email: "owner@example.com", Name: "Owner", Role: "admin"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.LoginSession("live", id, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO sessions(hash,expires_at,user_id) VALUES('expired',1,?)", id); err != nil {
		t.Fatal(err)
	}
	if err = s.PrunePrivateData(ctx, time.Now().Unix(), 90, 365); err != nil {
		t.Fatal(err)
	}
	user, err := s.SessionUser("live")
	if err != nil || user.ID != id || user.Name != "Owner" {
		t.Fatal(user, err)
	}
	if _, err = s.SessionUser("expired"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = s.DB.Exec("UPDATE users SET blocked=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SessionUser("live"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("blocked session accepted", err)
	}
	if _, err = s.UserByEmail("owner@example.com"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("blocked account accepted", err)
	}
}
func TestBatchMediaPreservesDetailsAndRevisions(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Media("image.png", "image/png", 1); err != nil {
		t.Fatal(err)
	}
	file, err := s.MediaFile("image.png")
	if err != nil {
		t.Fatal(err)
	}
	file.Alt = "Alternative"
	file.Caption = "Caption"
	if err = s.SaveMediaDetails(file); err != nil {
		t.Fatal(err)
	}
	single, err := s.MediaFile(file.Name)
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.MediaFiles([]string{file.Name, file.Name, "missing.png"})
	if err != nil || len(files) != 1 || files[file.Name] != single {
		t.Fatal(files, err)
	}
}
func TestModuleContentDoesNotLoadScreenshot(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := ModulePackage{ID: "preview", Manifest: `{}`, Template: "<p>Block</p>", CSS: "p{}", Settings: `{}`, Screenshot: make([]byte, 2<<20), ScreenshotMIME: "image/png"}
	if err = s.InstallModulePackage(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	full, err := s.ModulePackage(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := s.ModuleContent(p.ID)
	if err != nil || len(content.Screenshot) != 0 || content.Template != full.Template || content.CSS != full.CSS || content.Settings != full.Settings || content.Enabled != full.Enabled {
		t.Fatal("incorrect runtime projection", err)
	}
	if len(full.Screenshot) != 2<<20 {
		t.Fatal("full package lost its screenshot")
	}
}
