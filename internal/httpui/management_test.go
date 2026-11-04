package httpui

import (
	"context"
	"gopress/internal/i18n"
	"gopress/internal/store"
	"strings"
	"testing"
)

func TestManagementNavigationToolsAndReadableAudit(t *testing.T) {
	s, app, b := seoApp(t)
	users := b.request("GET", "/admin/users", "", "", false)
	status(t, users, 200)
	if strings.Contains(users.Body.String(), `href="/admin/account"`) || !strings.Contains(users.Body.String(), "Роли и доступ") {
		t.Fatal("duplicated administrator profile or missing access guide")
	}
	account := b.request("GET", "/admin/account", "", "", false)
	status(t, account, 303)
	if account.Header().Get("Location") != "/admin/users/1/edit" {
		t.Fatal(account.Header())
	}
	u, err := s.User(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.Service.Save(store.WithActor(context.Background(), u), store.Page{Title: "Journal target", Slug: "journal-target", Document: document}, false); err != nil {
		t.Fatal(err)
	}
	backups := b.request("GET", "/admin/tools", "", "", false)
	status(t, backups, 200)
	if !strings.Contains(backups.Body.String(), `id="backup-create"`) || strings.Contains(backups.Body.String(), `class="diagnostics-grid"`) {
		t.Fatal("tools tasks still mixed")
	}
	system := b.request("GET", "/admin/tools?tab=system", "", "", false)
	status(t, system, 200)
	if !strings.Contains(system.Body.String(), `class="diagnostics-grid"`) || strings.Contains(system.Body.String(), `id="backup-create"`) {
		t.Fatal("system tab mixes backups")
	}
	audit := b.request("GET", "/admin/tools?tab=audit", "", "", false)
	status(t, audit, 200)
	for _, want := range []string{"Создание материала", "Journal target", "owner@example.com", "audit-retention"} {
		if !strings.Contains(audit.Body.String(), want) {
			t.Fatal("audit missing", want)
		}
	}
	if strings.Contains(audit.Body.String(), "content.create") {
		t.Fatal("raw audit action displayed")
	}
	if _, err = s.DB.Exec("DELETE FROM audit_events"); err != nil {
		t.Fatal(err)
	}
	empty := b.request("GET", "/admin/tools?tab=audit", "", "", false)
	status(t, empty, 200)
	if !strings.Contains(empty.Body.String(), "Событий пока нет") {
		t.Fatal("empty audit is unexplained")
	}
}

func TestAuditLegacyObjectsRemainReadable(t *testing.T) {
	ctx := i18n.WithLanguage(context.Background(), "ru")
	for _, item := range []struct {
		event store.AuditEvent
		want  string
	}{
		{store.AuditEvent{Action: "user.password", Object: "1"}, i18n.T(ctx, "User") + " #1"},
		{store.AuditEvent{Action: "content.move", Object: "42"}, i18n.T(ctx, "Content") + " #42"},
		{store.AuditEvent{Action: "forms.export", Object: "7"}, i18n.T(ctx, "Form") + " #7"},
		{store.AuditEvent{Action: "content.delete", Object: `{"id":2,"title":"Deleted title","kind":"page"}`}, "Deleted title · #2"},
	} {
		if got := auditObject(ctx, item.event); got != item.want {
			t.Fatal(got, item.want)
		}
	}
	if got := auditDate(ctx, 1, "UTC"); got != "01.01.1970 00:00:01 UTC" {
		t.Fatal(got)
	}
}

func TestAuditActionsUseSelectedLanguage(t *testing.T) {
	for _, language := range []string{"en", "ru", "pl", "de", "uk", "it", "es", "cs", "zh", "ko", "ja", "fr", "pt", "tr"} {
		ctx := i18n.WithLanguage(context.Background(), language)
		for action, source := range map[string]string{
			"content.create":               "Content creation",
			"POST /admin/settings/general": "Site settings changes",
			"user.2fa_enabled":             "Enable 2FA",
			"unknown.action":               "Admin action",
		} {
			if language != "en" && i18n.Catalog(language)[source] == "" {
				t.Fatal("missing audit translation", language, source)
			}
			if got := auditAction(ctx, action); got != i18n.T(ctx, source) {
				t.Fatal("untranslated audit action", language, action, got)
			}
		}
	}
}
