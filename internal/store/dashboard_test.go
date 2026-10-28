package store

import (
	"context"
	"testing"
)

func TestDashboardUsesActiveMetadataAndActualUpdateOrder(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	create := func(kind, slug string, publish bool) Page {
		t.Helper()
		p, e := s.Save(ctx, Page{Kind: kind, Title: slug, Slug: slug, Document: `{"type":"doc"}`}, publish)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	page := create("page", "about", true)
	create("post", "draft", false)
	scheduled := create("post", "scheduled", true)
	trashed := create("page", "trashed", false)
	if err = s.Schedule(ctx, scheduled.ID, scheduled.Version, 2000); err != nil {
		t.Fatal(err)
	}
	scheduled, err = s.Page(scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	scheduled.Title = "Later draft"
	if _, err = s.Save(ctx, scheduled, false); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, trashed.ID, trashed.Version, "trash"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE pages SET updated_at=CASE WHEN id=? THEN 1000 ELSE 100 END", page.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Media("file.png", "image/png", 2048); err != nil {
		t.Fatal(err)
	}
	d, err := s.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	if d.Pages != (PageSummary{Total: 1, Published: 1}) || d.Posts != (PageSummary{Total: 2, Published: 1, Drafts: 1}) || d.Scheduled != 1 || d.Trashed != 1 || d.MediaCount != 1 || d.MediaBytes != 2048 {
		t.Fatalf("incorrect counts: %+v", d)
	}
	if len(d.Recent) != 3 || d.Recent[0].ID != page.ID || len(d.Upcoming) != 1 || d.Upcoming[0].ID != scheduled.ID || d.Upcoming[0].Title != "scheduled" {
		t.Fatalf("wrong selection: %+v", d)
	}
	for _, p := range append(d.Recent, d.Upcoming...) {
		if p.Document != "" || p.TrashedAt != 0 {
			t.Fatal("loaded document or trashed item", p)
		}
	}
}
