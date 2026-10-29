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
	if _, err = s.DB.Exec("DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; PRAGMA user_version=14"); err != nil {
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
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 15 {
		t.Fatal(version, err)
	}
}
