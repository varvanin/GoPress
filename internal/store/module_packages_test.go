package store

import "testing"

func TestModulePackagesUpgradeFromThirteen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE modules SET enabled=1 WHERE id='faq'; INSERT INTO faq_groups(name,items) VALUES('Existing FAQ','[]'); DROP TABLE module_collection_media; DROP TABLE module_collections; DELETE FROM modules WHERE id!='faq'; DROP TABLE module_packages; PRAGMA user_version=13"); err != nil {
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
	packages, err := s.ModulePackages()
	if err != nil || len(packages) != 0 {
		t.Fatal(packages, err)
	}
	enabled, err := s.FAQEnabled()
	if err != nil || !enabled {
		t.Fatal(enabled, err)
	}
	groups, err := s.FAQGroups()
	if err != nil || len(groups) != 1 || groups[0].Name != "Existing FAQ" {
		t.Fatal(groups, err)
	}
}
