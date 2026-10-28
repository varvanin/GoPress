package store

import (
	"context"
	"testing"
)

func TestHierarchyMovesPublishedChildrenAndSearchSnapshots(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	parent, err := s.Save(ctx, Page{Title: "Parent", Slug: "company", Document: lifecycleDocument, Kind: "page"}, true)
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Save(ctx, Page{Title: "Child", Slug: "contact", Document: lifecycleDocument, Kind: "page", ParentID: parent.ID}, true)
	if err != nil || child.PublishedSlug != "company/contact" {
		t.Fatal(child, err)
	}
	parent.Slug = "business"
	parent, err = s.Save(ctx, parent, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public("company/contact"); err != nil {
		t.Fatal("draft path changed child", err)
	}
	if err = s.Publish(ctx, parent.ID, parent.Version); err != nil {
		t.Fatal(err)
	}
	moved, err := s.Public("business/contact")
	if err != nil || moved.ID != child.ID {
		t.Fatal(moved, err)
	}
	if target, err := s.Redirect("company/contact"); err != nil || target != "business/contact" {
		t.Fatal("missing direct redirect", target, err)
	}
	current, _ := s.Page(child.ID)
	if current.Version <= child.Version || current.Slug != "business/contact" {
		t.Fatal("child draft/version not moved", current)
	}
	parent, _ = s.Page(parent.ID)
	parent.ParentID = child.ID
	if _, err = s.Save(ctx, parent, false); err != ErrConflict {
		t.Fatal("cycle accepted", err)
	}
	draft, _ := s.Page(child.ID)
	draft.Title = "HIDDEN DRAFT"
	if _, err = s.Save(ctx, draft, false); err != nil {
		t.Fatal(err)
	}
	results, total, err := s.SearchPublic("HIDDEN", 1)
	if err != nil || total != 0 {
		t.Fatal("draft entered search", results, err)
	}
	results, total, err = s.SearchPublic("Child", 1)
	if err != nil || total != 1 || results[0].Slug != "business/contact" {
		t.Fatal(results, total, err)
	}
	current, _ = s.Page(child.ID)
	parent, _ = s.Page(parent.ID)
	if err = s.ChangeContent(ctx, parent.ID, parent.Version, "trash"); err != ErrConflict {
		t.Fatal("trashed parent of published child", err)
	}
	if err = s.ChangeContents(ctx, "page", "trash", []Page{parent, current}); err != nil {
		t.Fatal("bulk hierarchy trash", err)
	}
	_, total, err = s.SearchPublic("Child", 1)
	if err != nil || total != 0 {
		t.Fatal("trash entered search", err)
	}
}
