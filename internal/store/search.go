package store

import (
	"strings"
	"unicode"
)

type SearchResult struct {
	ID                         int64
	Title, Slug, Snippet, Kind string
}

func (s *Store) SearchPublic(query string, page int) ([]SearchResult, int, error) {
	if page < 1 || page > 1000000 || len([]rune(query)) > 200 {
		return nil, 0, ErrConflict
	}
	terms := strings.Fields(query)
	if len(terms) > 10 {
		return nil, 0, ErrConflict
	}
	if len(terms) == 0 || strings.IndexFunc(query, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
		return []SearchResult{}, 0, nil
	}
	for i, term := range terms {
		terms[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	match := strings.Join(terms, " AND ")
	const from = ` FROM search_index JOIN pages p ON p.id=search_index.rowid WHERE search_index MATCH ? AND p.published_slug IS NOT NULL AND p.trashed_at=0`
	var total int
	if err := s.DB.QueryRow("SELECT count(*)"+from, match).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT p.id,p.published_title,p.published_slug,snippet(search_index,1,'','','…',24),p.kind"+from+" ORDER BY bm25(search_index,5.0,1.0),p.id DESC LIMIT 20 OFFSET ?", match, (page-1)*20)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		if err = rows.Scan(&r.ID, &r.Title, &r.Slug, &r.Snippet, &r.Kind); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}
func (s *Store) RebuildSearch() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM search_index"); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO search_index(rowid,title,body,slug) SELECT id,published_title,(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(pages.published_document) WHERE key='text' AND type='text'),published_slug FROM pages WHERE published_slug IS NOT NULL AND trashed_at=0`); err != nil {
		return err
	}
	return tx.Commit()
}
