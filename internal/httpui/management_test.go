package httpui

import (
	"context"
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
	ctx := context.Background()
	for _, item := range []struct {
		event store.AuditEvent
		want  string
	}{
		{store.AuditEvent{Action: "user.password", Object: "1"}, "Пользователь #1"},
		{store.AuditEvent{Action: "content.move", Object: "42"}, "Материал #42"},
		{store.AuditEvent{Action: "forms.export", Object: "7"}, "Форма #7"},
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
