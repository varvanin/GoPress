package httpui

import (
	"bytes"
	"context"
	"gopress/internal/store"
	"strings"
	"testing"
	"time"
)

func TestDashboardShowsUsefulDataAndSiteTimezone(t *testing.T) {
	at := time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC).Unix()
	data := store.DashboardData{Pages: store.PageSummary{Total: 2, Published: 1, Drafts: 1}, Scheduled: 1, Trashed: 3, MediaCount: 4, MediaBytes: 2 << 20, Recent: []store.Page{{ID: 1, Kind: "post", Title: "Recent story", Slug: "story", UpdatedAt: at}}, Upcoming: []store.Page{{ID: 1, Title: "Scheduled story", ScheduledAt: at}}}
	var out bytes.Buffer
	if err := Dashboard(store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "Europe/Warsaw"}, "csrf", data, store.Appearance{SEO: store.SEO{NoIndex: true}}, "Минимал", nil).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, marker := range []string{"Последние изменения", "Ближайшие публикации", "Recent story", "Scheduled story", "26.10.2026 19:00", "Europe/Warsaw", "https://example.com", "Минимал", "Запрещена", "2.0 MiB"} {
		if !strings.Contains(body, marker) {
			t.Errorf("missing %s", marker)
		}
	}
	if strings.Contains(body, "gp-content-bar") || strings.Contains(body, " style=") {
		t.Fatal("old chart or inline styles")
	}
}
