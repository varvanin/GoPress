package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestContentKindsAndMigration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	page, err := s.Save(ctx, Page{Title: "About", Slug: "about", Document: `{"type":"doc"}`}, true)
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.Save(ctx, Page{Kind: "post", Title: "Public post", Slug: "news", Document: `{"type":"doc"}`}, true)
	if err != nil {
		t.Fatal(err)
	}
	post.Title = "PRIVATE"
	post.Slug = "private"
	if _, err = s.Save(ctx, post, false); err != nil {
		t.Fatal(err)
	}
	pages, _, err := s.ListContent("page", "", false, 100, 0)
	if err != nil || len(pages) != 1 || pages[0].ID != page.ID {
		t.Fatalf("pages: %+v %v", pages, err)
	}
	posts, _, err := s.ListContent("post", "", false, 100, 0)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts: %+v %v", posts, err)
	}
	public, err := s.PublishedContent("post")
	if err != nil || len(public) != 1 || public[0].Title != "Public post" || public[0].Slug != "news" {
		t.Fatalf("public snapshot: %+v %v", public, err)
	}
	if _, err = s.DB.Exec("DELETE FROM routes WHERE page_id=?; DELETE FROM pages WHERE id=?", post.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	// Recreate the previous schema to exercise an actual upgrade on disk.
	if _, err = s.DB.Exec("DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; DROP TABLE settings; DROP TABLE form_entries; DROP TABLE forms; DROP TRIGGER search_pages_insert; DROP TRIGGER search_pages_update; DROP TRIGGER search_pages_delete; DROP TABLE search_index; DROP TABLE password_resets; DROP TABLE audit_events; DROP INDEX sessions_user; ALTER TABLE sessions DROP COLUMN user_id; DROP INDEX pages_owner; DROP INDEX pages_parent; DROP INDEX pages_published_parent; ALTER TABLE pages DROP COLUMN owner_id; ALTER TABLE pages DROP COLUMN parent_id; ALTER TABLE pages DROP COLUMN sort_order; ALTER TABLE pages DROP COLUMN template; ALTER TABLE pages DROP COLUMN published_parent_id; ALTER TABLE pages DROP COLUMN published_sort_order; ALTER TABLE pages DROP COLUMN published_template; ALTER TABLE media DROP COLUMN owner_id; ALTER TABLE revisions DROP COLUMN parent_id; ALTER TABLE revisions DROP COLUMN sort_order; ALTER TABLE revisions DROP COLUMN template; ALTER TABLE routes DROP COLUMN redirect; DROP INDEX pages_sitemap; DROP TABLE appearance; ALTER TABLE pages DROP COLUMN seo; ALTER TABLE pages DROP COLUMN published_seo; ALTER TABLE pages DROP COLUMN published_updated_at; ALTER TABLE revisions DROP COLUMN seo; DROP TABLE media_details; DROP TABLE menus; DROP TABLE terms; DROP INDEX pages_schedule; DROP INDEX pages_blog; ALTER TABLE pages DROP COLUMN term_ids; ALTER TABLE pages DROP COLUMN published_term_ids; ALTER TABLE pages DROP COLUMN scheduled_at; ALTER TABLE pages DROP COLUMN scheduled_version; DROP TABLE revisions; DROP INDEX pages_content; ALTER TABLE pages DROP COLUMN created_at; ALTER TABLE pages DROP COLUMN updated_at; ALTER TABLE pages DROP COLUMN published_at; ALTER TABLE pages DROP COLUMN trashed_at; ALTER TABLE pages DROP COLUMN kind; PRAGMA user_version=1"); err != nil {
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
	migrated, err := s.Page(page.ID)
	if err != nil || migrated.Kind != "page" || migrated.Title != "About" || !migrated.Published {
		t.Fatalf("migration lost page: %+v %v", migrated, err)
	}
	if err = CheckFile(filepath.Join(dir, "site.db")); err != nil {
		t.Fatal(err)
	}
}
