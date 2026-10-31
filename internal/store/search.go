package store

import (
	"net/url"
	"strings"
	"unicode"
)

type SearchResult struct {
	URL                        string
	ID                         int64
	Title, Slug, Snippet, Kind string
}

func (s *Store) SearchPublic(query string, page int, languages ...string) ([]SearchResult, int, error) {
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
	language := ""
	if len(languages) > 0 {
		language = languages[0]
	}
	const original = `SELECT p.id,p.published_title AS title,p.published_slug AS slug,snippet(search_index,1,'','','…',24) AS snippet,p.kind,'' AS language,bm25(search_index,5.0,1.0) AS rank FROM search_index JOIN pages p ON p.id=search_index.rowid WHERE search_index MATCH ? AND p.published_slug IS NOT NULL AND p.trashed_at=0 AND (?='' OR coalesce(nullif(json_extract(p.published_seo,'$.language'),''),'ru')=?)`
	const translated = `SELECT p.id,t.title,p.published_slug,snippet(translation_search,3,'','','…',24),p.kind,t.language,bm25(translation_search,0,0,5.0,1.0) FROM translation_search t JOIN pages p ON p.id=t.page_id WHERE translation_search MATCH ? AND t.language=? AND ?<>'' AND p.published_slug IS NOT NULL AND p.trashed_at=0`
	from := " FROM (" + original + " UNION ALL " + translated + ")"
	args := []any{match, language, language, match, language, language}
	var total int
	if err := s.DB.QueryRow("SELECT count(*)"+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT id,title,slug,snippet,kind,language"+from+" ORDER BY rank,id DESC LIMIT 20 OFFSET ?", append(args, (page-1)*20)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		var translated string
		if err = rows.Scan(&r.ID, &r.Title, &r.Slug, &r.Snippet, &r.Kind, &translated); err != nil {
			return nil, 0, err
		}
		r.URL = "/" + r.Slug
		if translated != "" {
			r.URL += "?lang=" + url.QueryEscape(translated)
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
	if _, err = tx.Exec(`DELETE FROM translation_search`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO translation_search(page_id,language,title,body) SELECT p.id,t.key,json_extract(t.value,'$.title'),(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(json_extract(t.value,'$.document')) WHERE key='text' AND type='text') FROM pages p,json_each(p.published_translations) t WHERE p.published_slug IS NOT NULL AND p.trashed_at=0`); err != nil {
		return err
	}
	return tx.Commit()
}
