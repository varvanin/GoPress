package store

func (s *Store) PageTemplateUsed(name string) (bool, error) {
	var used bool
	err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pages WHERE template=? OR published_template=?) OR EXISTS(SELECT 1 FROM revisions WHERE template=?)`, name, name, name).Scan(&used)
	return used, err
}
