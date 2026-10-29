package store

import "testing"

func TestFAQUpgradeFromTwelve(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; DROP TABLE faq_groups; DROP TABLE modules; PRAGMA user_version=12"); err != nil {
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
	enabled, err := s.FAQEnabled()
	if err != nil || enabled {
		t.Fatal("FAQ must start disabled", enabled, err)
	}
	groups, err := s.FAQGroups()
	if err != nil || len(groups) != 0 {
		t.Fatal(groups, err)
	}
}
