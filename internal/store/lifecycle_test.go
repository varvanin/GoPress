package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const lifecycleDocument = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Private draft"}]}]}`

func TestContentLifecycleAndRevisionRestore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	p, err := s.Save(ctx, Page{Kind: "post", Slug: "story", Title: "Public", Document: lifecycleDocument}, true)
	if err != nil {
		t.Fatal(err)
	}
	original := p
	p.Title = "Draft"
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.CreatedAt == 0 || p.UpdatedAt == 0 || p.PublishedAt == 0 {
		t.Fatal("missing dates")
	}
	if err = s.ChangeContent(ctx, p.ID, original.Version, "trash"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale trash: %v", err)
	}
	public, err := s.Public("story")
	if err != nil || public.Title != "Public" {
		t.Fatal("draft or stale trash changed public version")
	}
	rev, err := s.Revision(p.ID, original.Version)
	if err != nil || rev.Title != "Public" {
		t.Fatal("lost original revision")
	}
	rev.Version = p.Version
	p, err = s.Save(ctx, rev, false)
	if err != nil || p.Title != "Public" {
		t.Fatalf("restore: %+v %v", p, err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version, "unpublish"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public("story"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("unpublished content remains public")
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Publish(ctx, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version, "trash"); err != nil {
		t.Fatal(err)
	}
	active, total, err := s.ListContent("post", "", false, 25, 0)
	if err != nil || total != 0 || len(active) != 0 {
		t.Fatal("trash appears in active content")
	}
	trash, total, err := s.ListContent("post", "", true, 25, 0)
	if err != nil || total != 1 || len(trash) != 1 {
		t.Fatal("missing trash")
	}
	p = trash[0]
	if _, err = s.Public("story"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("trash remains public")
	}
	p.Document = lifecycleDocument
	if _, err = s.Save(ctx, p, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("saved trash: %v", err)
	}
	if err = s.ChangeContent(ctx, p.ID, p.Version, "restore"); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page(p.ID)
	if err != nil || p.TrashedAt != 0 || p.Published {
		t.Fatal("restoring trash must leave a draft")
	}
	revisions, err := s.Revisions(p.ID)
	if err != nil || len(revisions) != 7 {
		t.Fatalf("revision history %+v %v", revisions, err)
	}
}

func TestContentSearchPaginationAndMetadata(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 28; i++ {
		title := fmt.Sprintf("Проект %d", i)
		if i == 0 {
			title = "100% проект"
		}
		if _, err = s.Save(context.Background(), Page{Slug: fmt.Sprintf("item-%d", i), Title: title, Document: lifecycleDocument}, false); err != nil {
			t.Fatal(err)
		}
	}
	first, total, err := s.ListContent("page", "проект", false, 25, 0)
	if err != nil || total != 28 || len(first) != 25 {
		t.Fatalf("first page: %d %d %v", len(first), total, err)
	}
	second, total, err := s.ListContent("page", "проект", false, 25, 25)
	if err != nil || len(second) != 3 || total != 28 || first[24].ID <= second[0].ID {
		t.Fatal("pagination loses order or overlaps")
	}
	for _, p := range first {
		if p.Document != "" {
			t.Fatal("list loads editor documents")
		}
	}
	found, total, err := s.ListContent("page", "100%", false, 25, 0)
	if err != nil || total != 1 || found[0].Title != "100% проект" {
		t.Fatal("search treats percent as wildcard")
	}
	_, total, err = s.ListContent("post", "", false, 25, 0)
	if err != nil || total != 0 {
		t.Fatal("mixed content kinds")
	}
	if _, _, err = s.ListContent("page", "", false, 0, 0); err == nil {
		t.Fatal("unbounded query")
	}
}

func TestV2UpgradePreservesPublishedData(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(context.Background(), Page{Kind: "post", Slug: "old-post", Title: "Existing", Document: lifecycleDocument}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec("DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; DROP TABLE settings; DROP TABLE form_entries; DROP TABLE forms; DROP TRIGGER search_pages_insert; DROP TRIGGER search_pages_update; DROP TRIGGER search_pages_delete; DROP TABLE search_index; DROP TABLE password_resets; DROP TABLE audit_events; DROP INDEX sessions_user; ALTER TABLE sessions DROP COLUMN user_id; DROP INDEX pages_owner; DROP INDEX pages_parent; DROP INDEX pages_published_parent; ALTER TABLE pages DROP COLUMN owner_id; ALTER TABLE pages DROP COLUMN parent_id; ALTER TABLE pages DROP COLUMN sort_order; ALTER TABLE pages DROP COLUMN template; ALTER TABLE pages DROP COLUMN published_parent_id; ALTER TABLE pages DROP COLUMN published_sort_order; ALTER TABLE pages DROP COLUMN published_template; ALTER TABLE media DROP COLUMN owner_id; ALTER TABLE revisions DROP COLUMN parent_id; ALTER TABLE revisions DROP COLUMN sort_order; ALTER TABLE revisions DROP COLUMN template; ALTER TABLE routes DROP COLUMN redirect; DROP INDEX pages_sitemap; DROP TABLE appearance; ALTER TABLE pages DROP COLUMN seo; ALTER TABLE pages DROP COLUMN published_seo; ALTER TABLE pages DROP COLUMN published_updated_at; ALTER TABLE revisions DROP COLUMN seo; DROP TABLE media_details; DROP TABLE menus; DROP TABLE terms; DROP INDEX pages_schedule; DROP INDEX pages_blog; ALTER TABLE pages DROP COLUMN term_ids; ALTER TABLE pages DROP COLUMN published_term_ids; ALTER TABLE pages DROP COLUMN scheduled_at; ALTER TABLE pages DROP COLUMN scheduled_version; DROP TABLE revisions; DROP INDEX pages_content; ALTER TABLE pages DROP COLUMN created_at; ALTER TABLE pages DROP COLUMN updated_at; ALTER TABLE pages DROP COLUMN published_at; ALTER TABLE pages DROP COLUMN trashed_at; PRAGMA user_version=2")
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
	got, err := s.Page(p.ID)
	if err != nil || got.Title != p.Title || got.Kind != "post" || got.Version != p.Version || !got.Published {
		t.Fatalf("upgrade %+v %v", got, err)
	}
	revisions, err := s.Revisions(p.ID)
	if err != nil || len(revisions) != 1 || revisions[0].Version != p.Version {
		t.Fatal("upgrade did not seed current revision")
	}
}

func BenchmarkContentList(b *testing.B) {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	document := strings.Replace(lifecycleDocument, "Private draft", strings.Repeat("x", 32768), 1)
	_, err = s.DB.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
 INSERT INTO pages(slug,version,title,document,kind,created_at,updated_at)
 SELECT 'item-'||x,1,'Проект '||x,?,'page',unixepoch(),unixepoch() FROM n`, document)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pages, total, err := s.ListContent("page", "", false, 25, 0)
		if err != nil || len(pages) != 25 || total != 1000 {
			b.Fatal(err)
		}
	}
}

func TestBulkContentAtomicRestoreAndDelete(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, kind := range []string{"page", "post"} {
		a, err := s.Save(ctx, Page{Kind: kind, Slug: kind + "-a", Title: "A", Document: lifecycleDocument}, true)
		if err != nil {
			t.Fatal(err)
		}
		b, err := s.Save(ctx, Page{Kind: kind, Slug: kind + "-b", Title: "B", Document: lifecycleDocument}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ChangeContent(ctx, a.ID, a.Version, "delete"); !errors.Is(err, ErrConflict) {
			t.Fatalf("deleted active content: %v", err)
		}
		stale := b
		stale.Version++
		if err = s.ChangeContents(ctx, kind, "trash", []Page{a, stale}); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		current, _ := s.Page(a.ID)
		if current.TrashedAt != 0 || current.Version != a.Version {
			t.Fatal("partial bulk change")
		}
		if err = s.ChangeContents(ctx, kind, "trash", []Page{a, b}); err != nil {
			t.Fatal(err)
		}
		a, _ = s.Page(a.ID)
		b, _ = s.Page(b.ID)
		if err = s.ChangeContents(ctx, kind, "restore", []Page{a, b}); err != nil {
			t.Fatal(err)
		}
		a, _ = s.Page(a.ID)
		b, _ = s.Page(b.ID)
		if a.TrashedAt != 0 || a.Published {
			t.Fatal("restore must leave draft")
		}
		if err = s.ChangeContents(ctx, kind, "trash", []Page{a, b}); err != nil {
			t.Fatal(err)
		}
		a, _ = s.Page(a.ID)
		b, _ = s.Page(b.ID)
		stale = b
		stale.Version++
		if err = s.ChangeContents(ctx, kind, "delete", []Page{a, stale}); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		if _, err = s.Page(a.ID); err != nil {
			t.Fatal("partial permanent deletion", err)
		}
		if err = s.ChangeContents(ctx, kind, "delete", []Page{a, b}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Page(a.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("content survived", err)
		}
		var count int
		if err = s.DB.QueryRow("SELECT (SELECT count(*) FROM revisions WHERE page_id=?)+(SELECT count(*) FROM routes WHERE page_id=?)", a.ID, a.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("orphaned routes/revisions", count, err)
		}
	}
}
