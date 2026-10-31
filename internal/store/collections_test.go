package store

import (
	"context"
	"testing"
)

func TestCollectionsUpgradeFromFourteen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetFAQEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DROP TABLE IF EXISTS content_reviews; DROP TABLE IF EXISTS manual_redirects; DROP TABLE IF EXISTS missing_paths; DROP TRIGGER IF EXISTS users_security_insert; DROP TABLE IF EXISTS session_details; DROP TABLE IF EXISTS user_recovery_codes; DROP TABLE IF EXISTS user_security; DROP TRIGGER IF EXISTS translations_search_insert; DROP TRIGGER IF EXISTS translations_search_update; DROP TRIGGER IF EXISTS translations_search_delete; DROP TABLE IF EXISTS translation_search; DROP TABLE IF EXISTS localized_menus; ALTER TABLE pages DROP COLUMN translations; ALTER TABLE pages DROP COLUMN published_translations; ALTER TABLE revisions DROP COLUMN translations; DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; PRAGMA user_version=14"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, kind := range []string{"faq", "gallery", "testimonials"} {
		enabled, err := s.ModuleEnabled(kind)
		if err != nil || enabled != (kind == "faq") {
			t.Fatal(kind, enabled, err)
		}
	}
	var version int
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal(version, err)
	}
}
