package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestContentAuditPreservesActorAndTitleAfterDeletion(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := WithActor(context.Background(), User{ID: 7, Role: "admin"})
	p, err := s.Save(ctx, Page{Title: "Original title", Slug: "audit", Document: `{"type":"doc"}`}, false)
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "Renamed title"
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version-1, "trash"); err != ErrConflict {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version, "trash"); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version, "delete"); err != nil {
		t.Fatal(err)
	}
	events, total, err := s.AuditEvents(1)
	if err != nil || total != 4 {
		t.Fatal(events, total, err)
	}
	for i, want := range []string{"content.delete", "content.trash", "content.update", "content.create"} {
		e := events[i]
		var target struct {
			ID          int64
			Title, Kind string
		}
		if err = json.Unmarshal([]byte(e.Object), &target); err != nil {
			t.Fatal(err)
		}
		title := "Renamed title"
		if i == 3 {
			title = "Original title"
		}
		if e.Action != want || e.UserID != 7 || target.ID != p.ID || target.Title != title || target.Kind != "page" {
			t.Fatal(e, target)
		}
	}
}
