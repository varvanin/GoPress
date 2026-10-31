package httpui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"gopress/internal/store"
)

func TestDashboardAttentionUsesCurrentFailuresAndHidesResolvedEvents(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	initial := b.request("GET", "/admin", "", "", false)
	status(t, initial, 200)
	if strings.Contains(initial.Body.String(), "data-dashboard-attention") {
		t.Fatal("invented attention items")
	}
	p, err := app.Service.Save(ctx, store.Page{Kind: "post", Title: "Failed scheduled post", Slug: "attention", Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Audit(ctx, "content.schedule_failed", fmt.Sprintf(`{"id":%d,"version":%d}`, p.ID, p.Version)); err != nil {
		t.Fatal(err)
	}
	data, err := s.Dashboard()
	if err != nil || data.FailedPublications != 1 {
		t.Fatal(data, err)
	}
	page := b.request("GET", "/admin", "", "", false)
	status(t, page, 200)
	if !strings.Contains(page.Body.String(), "data-dashboard-attention") {
		t.Fatal("failure missing from overview")
	}
	p.Title = "Corrected draft"
	if _, err = app.Service.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	data, err = s.Dashboard()
	if err != nil || data.FailedPublications != 0 {
		t.Fatal("stale failure retained", data, err)
	}
}
func TestBackupScheduleSettingsProtectCSRFAndConcurrentChanges(t *testing.T) {
	s, _, b := seoApp(t)
	values := url.Values{"version": {"0"}, "enabled": {"true"}, "hour": {"3"}, "keep": {"5"}}
	status(t, adminHTML(b, "POST", "/admin/tools/backups/schedule", values, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/tools/backups/schedule", values, false, false, true), 303)
	state, err := s.BackupSchedule()
	if err != nil || !state.Enabled || state.Hour != 3 || state.Keep != 5 {
		t.Fatal(state, err)
	}
	values.Set("keep", "6")
	conflict := adminHTML(b, "POST", "/admin/tools/backups/schedule", values, true, false, true)
	status(t, conflict, 409)
	if !strings.Contains(conflict.Body.String(), `value="6"`) {
		t.Fatal("conflict lost entered retention")
	}
	if err = s.ClaimScheduledBackup(context.Background(), "2027-01-15", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	data, err := s.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	app := b.app
	items, err := app.dashboardAttention(data, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Label == "Проверьте автоматическое резервное копирование" {
			found = true
		}
	}
	if !found {
		t.Fatal("failed backup missing from attention")
	}
}
