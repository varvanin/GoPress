package store

import (
	"testing"
)

func TestSiteMigrationFromFour(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO media(name,mime,size) VALUES('legacy.png','image/png',1); DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; DROP TABLE settings; DROP TABLE form_entries; DROP TABLE forms; DROP TRIGGER search_pages_insert; DROP TRIGGER search_pages_update; DROP TRIGGER search_pages_delete; DROP TABLE search_index; DROP TABLE password_resets; DROP TABLE audit_events; DROP INDEX sessions_user; ALTER TABLE sessions DROP COLUMN user_id; DROP INDEX pages_owner; DROP INDEX pages_parent; DROP INDEX pages_published_parent; ALTER TABLE pages DROP COLUMN owner_id; ALTER TABLE pages DROP COLUMN parent_id; ALTER TABLE pages DROP COLUMN sort_order; ALTER TABLE pages DROP COLUMN template; ALTER TABLE pages DROP COLUMN published_parent_id; ALTER TABLE pages DROP COLUMN published_sort_order; ALTER TABLE pages DROP COLUMN published_template; ALTER TABLE media DROP COLUMN owner_id; ALTER TABLE revisions DROP COLUMN parent_id; ALTER TABLE revisions DROP COLUMN sort_order; ALTER TABLE revisions DROP COLUMN template; ALTER TABLE routes DROP COLUMN redirect; DROP INDEX pages_sitemap; DROP TABLE appearance; ALTER TABLE pages DROP COLUMN seo; ALTER TABLE pages DROP COLUMN published_seo; ALTER TABLE pages DROP COLUMN published_updated_at; ALTER TABLE revisions DROP COLUMN seo; DROP TABLE media_details; DROP TABLE menus; PRAGMA user_version=4`); err != nil {
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
	var version int
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 15 {
		t.Fatal(version, err)
	}
	if _, err = s.Menu("header"); err != nil {
		t.Fatal(err)
	}
	files, total, err := s.ListMedia("legacy", 1)
	if err != nil || total != 1 || files[0].Name != "legacy.png" {
		t.Fatalf("%+v %d %v", files, total, err)
	}
	for i := 0; i < 25; i++ {
		if err = s.Media(string(rune('a'+i))+".png", "image/png", 1); err != nil {
			t.Fatal(err)
		}
	}
	files, total, err = s.ListMedia("", 2)
	if err != nil || total != 26 || len(files) != 2 {
		t.Fatalf("pagination %d %d %v", total, len(files), err)
	}
}
