package httpui

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestToolsBackupRestoreAndMaintenance(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Title: "Preserved", Slug: "restore-me", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(b, "GET", "/admin/tools", nil, false, false, false), 200)
	status(t, adminHTML(b, "POST", "/admin/tools/backups/create", nil, true, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/tools/backups/create", nil, true, false, true), 200)
	backups, err := app.Service.Backups()
	if err != nil || len(backups) != 1 {
		t.Fatal(backups, err)
	}
	p.Title = "After backup"
	if _, err = app.Service.Save(ctx, p, true); err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(b, "POST", "/admin/tools/backups/restore", url.Values{"backup": {backups[0].ID}, "confirm": {"RESTORE"}}, true, false, true), 400)
	status(t, adminHTML(b, "POST", "/admin/tools/maintenance", url.Values{"enabled": {"true"}}, true, false, true), 303)
	maintenance := b.request("GET", "/restore-me", "", "", false)
	status(t, maintenance, 503)
	for _, marker := range []string{"Технические работы", "/assets/gopress.svg", "auth-card", "Обновить страницу", `href="/restore-me"`} {
		if !strings.Contains(maintenance.Body.String(), marker) {
			t.Fatal("maintenance page missing", marker)
		}
	}
	if maintenance.Header().Get("Retry-After") != "300" || maintenance.Header().Get("Cache-Control") != "no-store" || !strings.Contains(maintenance.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatal("unsafe maintenance headers", maintenance.Header())
	}
	status(t, b.request("GET", "/assets/admin.css", "", "", false), 200)
	status(t, b.request("GET", "/assets/gopress.svg", "", "", false), 200)
	status(t, b.request("GET", "/login", "", "", false), 200)
	head := b.request("HEAD", "/restore-me", "", "", false)
	status(t, head, 503)
	if head.Body.Len() != 0 {
		t.Fatal("HEAD returned a body")
	}
	status(t, b.request("GET", "/admin/tools?tab=system", "", "", false), 200)
	status(t, adminHTML(b, "POST", "/admin/pages/save", url.Values{}, true, false, true), 503)
	status(t, adminHTML(b, "POST", "/admin/tools/backups/restore", url.Values{"backup": {backups[0].ID}, "confirm": {"RESTORE"}}, true, false, true), 303)
	restored, err := s.Page(p.ID)
	if err != nil || restored.Title != "Preserved" {
		t.Fatal(restored, err)
	}
	if ok, _ := s.Authorized("missing"); ok {
		t.Fatal("session was not revoked")
	}
	status(t, adminHTML(b, "GET", "/admin/tools", nil, false, false, false), 303)
	if err = app.Service.SetMaintenance(false); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", "/restore-me", "", "", false), 200)
}

func TestPublicSearchUsesPublishedSnapshot(t *testing.T) {
	_, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Title: "Public needle", Slug: "public-search", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "Secret draftword"
	if _, err = app.Service.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/search?q=needle", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Public needle") || strings.Contains(response.Body.String(), "Secret draftword") {
		t.Fatal("wrong search snapshot", response.Body.String())
	}
	response = b.request("GET", "/search?q=draftword", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), `class="search-count">Найдено: 0</p>`) || strings.Contains(response.Body.String(), "Secret draftword") {
		t.Fatal("draft search result leaked")
	}
}
