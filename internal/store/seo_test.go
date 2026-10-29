package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSEOIsVersionedAndScheduled(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	p, err := s.Save(ctx, Page{Kind: "post", Title: "Story", Slug: "story", Document: `{"type":"doc"}`, SEO: SEO{Title: "Public SEO", Description: "Public description"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := p.Version
	if _, err = s.DB.Exec(`UPDATE pages SET published_at=100,published_updated_at=200 WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	p.SEO = SEO{Title: "Scheduled SEO", NoIndex: true}
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Schedule(ctx, p.ID, p.Version, 300); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.SEO = SEO{Title: "Later draft"}
	if _, err = s.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	public, err := s.Public("story")
	if err != nil || public.SEO.Title != "Public SEO" || public.SEO.NoIndex || public.PublishedUpdatedAt != 200 {
		t.Fatalf("public changed %+v %v", public, err)
	}
	if n, err := s.PublishDue(ctx, 400); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	public, err = s.Public("story")
	if err != nil || public.SEO.Title != "Scheduled SEO" || !public.SEO.NoIndex || public.PublishedAt != 100 || public.PublishedUpdatedAt != 400 {
		t.Fatalf("schedule %+v %v", public, err)
	}
	draft, err := s.Page(p.ID)
	if err != nil || draft.SEO.Title != "Later draft" {
		t.Fatalf("draft lost %+v %v", draft, err)
	}
	revision, err := s.Revision(p.ID, oldVersion)
	if err != nil || revision.SEO.Title != "Public SEO" {
		t.Fatalf("revision %+v %v", revision, err)
	}
	if n, err := s.SitemapCount(); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if posts, err := s.FeedPosts(); err != nil || len(posts) != 0 {
		t.Fatal(posts, err)
	}
}
func TestSEOMigrationFromFive(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(context.Background(), Page{Title: "Legacy", Slug: "legacy", Document: `{"type":"doc"}`}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`ALTER TABLE pages DROP COLUMN translations; ALTER TABLE pages DROP COLUMN published_translations; ALTER TABLE revisions DROP COLUMN translations; DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; DROP TABLE settings; DROP TABLE form_entries; DROP TABLE forms; DROP TRIGGER search_pages_insert; DROP TRIGGER search_pages_update; DROP TRIGGER search_pages_delete; DROP TABLE search_index; DROP TABLE password_resets; DROP TABLE audit_events; DROP INDEX sessions_user; ALTER TABLE sessions DROP COLUMN user_id; DROP INDEX pages_owner; DROP INDEX pages_parent; DROP INDEX pages_published_parent; ALTER TABLE pages DROP COLUMN owner_id; ALTER TABLE pages DROP COLUMN parent_id; ALTER TABLE pages DROP COLUMN sort_order; ALTER TABLE pages DROP COLUMN template; ALTER TABLE pages DROP COLUMN published_parent_id; ALTER TABLE pages DROP COLUMN published_sort_order; ALTER TABLE pages DROP COLUMN published_template; ALTER TABLE media DROP COLUMN owner_id; ALTER TABLE revisions DROP COLUMN parent_id; ALTER TABLE revisions DROP COLUMN sort_order; ALTER TABLE revisions DROP COLUMN template; ALTER TABLE routes DROP COLUMN redirect; DROP INDEX pages_sitemap; DROP TABLE appearance; ALTER TABLE pages DROP COLUMN seo; ALTER TABLE pages DROP COLUMN published_seo; ALTER TABLE pages DROP COLUMN published_updated_at; ALTER TABLE revisions DROP COLUMN seo; PRAGMA user_version=5`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	public, err := s.Public("legacy")
	if err != nil || public.ID != p.ID || public.PublishedUpdatedAt != p.PublishedAt || public.SEO != (SEO{}) {
		t.Fatalf("migration %+v %v", public, err)
	}
	if _, err = s.Revision(p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	a, err := s.Appearance()
	if err != nil || a.Version != 0 {
		t.Fatalf("appearance %+v %v", a, err)
	}
	a.Theme = "minimal"
	if err = s.SaveAppearance(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAppearance(context.Background(), a); err != ErrConflict {
		t.Fatal("stale appearance accepted", err)
	}
	if err = CheckFile(filepath.Join(dir, "site.db")); err != nil {
		t.Fatal(err)
	}
}
