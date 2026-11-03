package service

import (
	"context"
	"errors"
	"testing"

	"gopress/internal/store"
)

func TestInstallationLanguageIsValidatedAndCommittedAtomically(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	site := store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}
	install := func(language string) error {
		return v.Install(ctx, v.InstallToken, site, "owner@example.com", "correct horse battery staple", language)
	}
	if err = install("xx"); !errors.Is(err, ErrInvalid) {
		t.Fatal("unsupported interface language accepted", err)
	}
	if _, err = s.DB.Exec("CREATE TRIGGER fail_installation BEFORE INSERT ON installation BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if err = install("fr"); err == nil {
		t.Fatal("installation failure ignored")
	}
	var language string
	var users int
	if err = s.DB.QueryRow("SELECT json_extract(value,'$.language') FROM settings WHERE id=1").Scan(&language); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if language != "ru" || users != 0 || v.Installed() {
		t.Fatal("failed installation was partially saved", language, users)
	}
	if _, err = s.DB.Exec("DROP TRIGGER fail_installation"); err != nil {
		t.Fatal(err)
	}
	if err = install("fr"); err != nil {
		t.Fatal(err)
	}
	settings, err := s.Settings()
	if err != nil || settings.Language != "fr" || settings.HomeMode != "posts" || settings.PostsPerPage != 12 {
		t.Fatal("installation settings were lost", settings, err)
	}
}
