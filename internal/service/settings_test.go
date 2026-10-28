package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestSettingsAndMailPersistence(t *testing.T) {
	t.Setenv("GOPRESS_SMTP_HOST", "")
	t.Setenv("GOPRESS_SMTP_FROM", "")
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
	if err = v.Install(ctx, v.InstallToken, store.Site{Title: "Original", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	settings, err := s.Settings()
	if err != nil || settings.Language != "ru" || settings.PostsPerPage != 12 {
		t.Fatal(settings, err)
	}
	original := settings
	for _, change := range []func(*store.Settings){func(s *store.Settings) { s.Site.BaseURL = "javascript:alert(1)" }, func(s *store.Settings) { s.Site.Timezone = "Invalid/Zone" }, func(s *store.Settings) { s.Language = "xx" }, func(s *store.Settings) { s.PostsPerPage = 101 }, func(s *store.Settings) { s.HomeMode = "page" }} {
		bad := settings
		change(&bad)
		if err = v.SaveSettings(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatal("accepted invalid settings", err)
		}
	}
	settings.Site.Title = "Updated"
	settings.Description = "Description"
	settings.Site.Timezone = "Europe/Warsaw"
	settings.Language = "pl"
	if err = v.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err = v.SaveSettings(ctx, original); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale settings overwrote values", err)
	}
	saved, _ := s.Settings()
	site, _ := s.Site()
	if site.Title != "Updated" || saved.Language != "pl" || saved.Description != "Description" {
		t.Fatal(site, saved)
	}
	mail := MailSettings{Enabled: true, Host: "smtp.example.com", Port: "587", From: "website@example.com", Username: "user", Password: "private-secret"}
	if err = v.SaveMailSettings(mail, false); err != nil {
		t.Fatal(err)
	}
	first, err := v.MailSettings()
	if err != nil || first.Password != "private-secret" || !first.Configured() {
		t.Fatal("mail not saved", err)
	}
	info, err := os.Stat(filepath.Join(s.Dir, "mail.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("mail permissions", err)
	}
	if err = v.SaveMailSettings(mail, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale mail overwritten", err)
	}
	first.Password = ""
	first.From = "changed@example.com"
	if err = v.SaveMailSettings(first, false); err != nil {
		t.Fatal(err)
	}
	second, _ := v.MailSettings()
	if second.Password != "private-secret" {
		t.Fatal("blank field erased password")
	}
	if err = v.SaveMailSettings(second, true); err != nil {
		t.Fatal(err)
	}
	third, _ := v.MailSettings()
	if third.Password != "" {
		t.Fatal("clear password ignored")
	}
	third.Host = "smtp.example.com\r\nMAIL FROM:bad"
	if err = v.SaveMailSettings(third, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted invalid SMTP host", err)
	}
	t.Setenv("GOPRESS_SMTP_HOST", "env.example.com")
	t.Setenv("GOPRESS_SMTP_FROM", "env@example.com")
	env, err := v.MailSettings()
	if err != nil || !env.Environment || env.Host != "env.example.com" {
		t.Fatal("environment precedence", err)
	}
	if err = v.SaveMailSettings(env, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("overwrote environment", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir, "mail.json"))
	if err != nil || strings.Contains(string(raw), "env.example.com") {
		t.Fatal("environment leaked into file", err)
	}
}
