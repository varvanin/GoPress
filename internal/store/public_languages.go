package store

func (s *Store) PublicLanguageAvailable(id int64, language string) (bool, error) {
	var available bool
	err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pages p WHERE id=? AND trashed_at=0 AND published_slug IS NOT NULL AND (coalesce(nullif(json_extract(published_seo,'$.language'),''),'ru')=? OR EXISTS(SELECT 1 FROM json_each(p.published_translations) WHERE key=?)))`, id, language, language).Scan(&available)
	return available, err
}
