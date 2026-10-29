package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestScheduledSnapshotSurvivesRestartAndLaterDraft(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, err := s.Save(ctx, Page{Kind: "post", Title: "Scheduled", Slug: "scheduled", Document: lifecycleDocument}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Schedule(ctx, p.ID, p.Version, 100); err != nil {
		t.Fatal(err)
	}
	scheduled, err := s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.ScheduledVersion != scheduled.Version {
		t.Fatal("schedule did not pin a revision")
	}
	p = scheduled
	p.Title = "PRIVATE LATER DRAFT"
	p.Slug = "private"
	p, err = s.Save(ctx, p, false)
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
	if n, err := s.PublishDue(ctx, 99); err != nil || n != 0 {
		t.Fatalf("early publication %d %v", n, err)
	}
	if n, err := s.PublishDue(ctx, 101); err != nil || n != 1 {
		t.Fatalf("publication %d %v", n, err)
	}
	public, err := s.Public("scheduled")
	if err != nil || public.Title != "Scheduled" {
		t.Fatalf("wrong snapshot %+v %v", public, err)
	}
	current, err := s.Page(p.ID)
	if err != nil || current.Title != "PRIVATE LATER DRAFT" || current.ScheduledAt != 0 {
		t.Fatal("scheduler overwrote the working draft")
	}
	if n, err := s.PublishDue(ctx, 102); err != nil || n != 0 {
		t.Fatal("published twice")
	}
	if _, err = s.Public("private"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("later draft leaked")
	}
	if _, err = s.Save(ctx, p, false); !errors.Is(err, ErrConflict) {
		t.Fatal("scheduler failed to invalidate stale forms")
	}
}

func TestTaxonomySnapshotsFilteringAndRemoval(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, term := range []Term{{Kind: "category", Name: "News"}, {Kind: "tag", Name: "Go"}} {
		if err = s.SaveTerm(ctx, term, false); err != nil {
			t.Fatal(err)
		}
	}
	terms, err := s.Terms(false)
	if err != nil || len(terms) != 2 {
		t.Fatal(err)
	}
	category, tag := terms[0], terms[1]
	p, err := s.Save(ctx, Page{Kind: "post", Title: "Post", Slug: "post", Document: lifecycleDocument, TermIDs: []int64{category.ID}}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.TermIDs = []int64{tag.ID}
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	public, err := s.Public("post")
	if err != nil || len(public.TermIDs) != 1 || public.TermIDs[0] != category.ID {
		t.Fatal("draft taxonomy leaked")
	}
	posts, total, err := s.PublicPosts(category.ID, 1)
	if err != nil || total != 1 || len(posts) != 1 {
		t.Fatal("public taxonomy filter failed")
	}
	_, total, err = s.ListContent("post", "", false, 25, 0, tag.ID)
	if err != nil || total != 1 {
		t.Fatal("admin taxonomy filter failed")
	}
	visible, err := s.Terms(true)
	if err != nil || len(visible) != 1 || visible[0].ID != category.ID {
		t.Fatal("private taxonomy appeared publicly")
	}
	category.Name = "Announcements"
	if err = s.SaveTerm(ctx, category, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveTerm(ctx, category, false); !errors.Is(err, ErrConflict) {
		t.Fatal("stale rename overwrote a term")
	}
	category.Version++
	if err = s.SaveTerm(ctx, category, true); err != nil {
		t.Fatal(err)
	}
	visible, err = s.Terms(true)
	if err != nil || len(visible) != 0 {
		t.Fatal("deleted taxonomy remains public")
	}
	active, err := s.ActiveTermIDs([]int64{category.ID, tag.ID})
	if err != nil || len(active) != 1 || active[0] != tag.ID {
		t.Fatal("deleted terms cannot be filtered from revisions")
	}
	p.TermIDs = []int64{category.ID}
	if _, err = s.Save(ctx, p, false); !errors.Is(err, ErrInvalidTerms) {
		t.Fatal("accepted removed taxonomy")
	}
}
func TestScheduleCancellationAndTrash(t *testing.T) {
	for _, action := range []string{"cancel_schedule", "trash", "unpublish"} {
		t.Run(action, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()
			p, err := s.Save(ctx, Page{Kind: "post", Title: "Draft", Slug: "draft", Document: lifecycleDocument}, false)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Schedule(ctx, p.ID, p.Version, 100); err != nil {
				t.Fatal(err)
			}
			p, err = s.Page(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ChangeContent(ctx, p.ID, p.Version, action); err != nil {
				t.Fatal(err)
			}
			if n, err := s.PublishDue(ctx, 101); err != nil || n != 0 {
				t.Fatal("cancelled or trashed job published")
			}
		})
	}
}

func TestV3BlogUpgradeKeepsContentAndRevisions(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(context.Background(), Page{Kind: "post", Title: "Old post", Slug: "old", Document: lifecycleDocument}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec("ALTER TABLE pages DROP COLUMN translations; ALTER TABLE pages DROP COLUMN published_translations; ALTER TABLE revisions DROP COLUMN translations; DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; DROP TABLE settings; DROP TABLE form_entries; DROP TABLE forms; DROP TRIGGER search_pages_insert; DROP TRIGGER search_pages_update; DROP TRIGGER search_pages_delete; DROP TABLE search_index; DROP TABLE password_resets; DROP TABLE audit_events; DROP INDEX sessions_user; ALTER TABLE sessions DROP COLUMN user_id; DROP INDEX pages_owner; DROP INDEX pages_parent; DROP INDEX pages_published_parent; ALTER TABLE pages DROP COLUMN owner_id; ALTER TABLE pages DROP COLUMN parent_id; ALTER TABLE pages DROP COLUMN sort_order; ALTER TABLE pages DROP COLUMN template; ALTER TABLE pages DROP COLUMN published_parent_id; ALTER TABLE pages DROP COLUMN published_sort_order; ALTER TABLE pages DROP COLUMN published_template; ALTER TABLE media DROP COLUMN owner_id; ALTER TABLE revisions DROP COLUMN parent_id; ALTER TABLE revisions DROP COLUMN sort_order; ALTER TABLE revisions DROP COLUMN template; ALTER TABLE routes DROP COLUMN redirect; DROP INDEX pages_sitemap; DROP TABLE appearance; ALTER TABLE pages DROP COLUMN seo; ALTER TABLE pages DROP COLUMN published_seo; ALTER TABLE pages DROP COLUMN published_updated_at; ALTER TABLE revisions DROP COLUMN seo; DROP TABLE media_details; DROP TABLE menus; DROP TABLE terms; DROP INDEX pages_schedule; DROP INDEX pages_blog; ALTER TABLE pages DROP COLUMN term_ids; ALTER TABLE pages DROP COLUMN published_term_ids; ALTER TABLE pages DROP COLUMN scheduled_at; ALTER TABLE pages DROP COLUMN scheduled_version; ALTER TABLE revisions DROP COLUMN term_ids; PRAGMA user_version=3")
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
	migrated, err := s.Page(p.ID)
	if err != nil || migrated.Title != p.Title || migrated.Version != p.Version || !migrated.Published || len(migrated.TermIDs) != 0 {
		t.Fatal("upgrade changed content")
	}
	revision, err := s.Revision(p.ID, p.Version)
	if err != nil || revision.Title != p.Title {
		t.Fatal("upgrade lost revision")
	}
}

func TestScheduledFailureDoesNotBlockLaterBatches(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	parent, err := s.Save(ctx, Page{Title: "Parent", Slug: "parent", Document: lifecycleDocument}, true)
	if err != nil {
		t.Fatal(err)
	}
	var pinned Page
	for i := 0; i < 12; i++ {
		child, e := s.Save(ctx, Page{Title: "Scheduled child", Slug: fmt.Sprintf("child-%d", i), ParentID: parent.ID, Document: lifecycleDocument}, false)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Schedule(ctx, child.ID, child.Version, 100); e != nil {
			t.Fatal(e)
		}
		pinned, e = s.Page(child.ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	post, err := s.Save(ctx, Page{Kind: "post", Title: "Independent", Slug: "independent", Document: lifecycleDocument}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Schedule(ctx, post.ID, post.Version, 101); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, parent.ID, parent.Version, "unpublish"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{10, 3, 0} {
		if n, e := s.PublishDue(ctx, 102); e != nil || n != want {
			t.Fatalf("processed=%d want=%d err=%v", n, want, e)
		}
	}
	published, err := s.Page(post.ID)
	if err != nil || !published.Published {
		t.Fatal("independent publication blocked", err)
	}
	failed, err := s.Page(pinned.ID)
	if err != nil || failed.Published || failed.ScheduledAt != 0 || failed.Document != pinned.Document {
		t.Fatal("failed job changed draft", err)
	}
	if _, err = s.Revision(pinned.ID, pinned.ScheduledVersion); err != nil {
		t.Fatal("pinned revision lost", err)
	}
	if failedJob, e := s.ScheduleFailed(failed.ID, failed.Version); e != nil || !failedJob {
		t.Fatal("failed job not visible", e)
	}
	if err = s.Schedule(ctx, failed.ID, failed.Version, 200); err != nil {
		t.Fatal(err)
	}
	retried, e := s.Page(failed.ID)
	if e != nil {
		t.Fatal(e)
	}
	if stale, e := s.ScheduleFailed(retried.ID, retried.Version); e != nil || stale {
		t.Fatal("rescheduled job still shows old failure", e)
	}
	var failures int
	if err = s.DB.QueryRow("SELECT count(*) FROM audit_events WHERE action='content.schedule_failed'").Scan(&failures); err != nil || failures != 12 {
		t.Fatal("missing individual errors", failures, err)
	}
}
