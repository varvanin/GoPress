package store

import (
	"context"
	"encoding/json"
	"strings"
)

type Appearance struct {
	Version         int64      `json:"-"`
	Theme           string     `json:"theme"`
	HomeTitle       string     `json:"home_title"`
	HomeDescription string     `json:"home_description"`
	SEO             SEO        `json:"seo"`
	SEOOptions      SEOOptions `json:"seo_options"`
}

func (s *Store) Appearance() (Appearance, error) {
	var a Appearance
	var raw string
	err := s.DB.QueryRow("SELECT version,value FROM appearance WHERE id=1").Scan(&a.Version, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &a)
	}
	return a, err
}
func (s *Store) SaveAppearance(ctx context.Context, a Appearance) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, "UPDATE appearance SET value=?,version=version+1 WHERE id=1 AND version=?", string(raw), a.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}

const publicIndexWhere = `trashed_at=0 AND published_slug IS NOT NULL AND coalesce(json_extract(published_seo,'$.noindex'),0)=0`

func sitemapFilter(options []SEOOptions) (string, []any) {
	where := publicIndexWhere + " AND (coalesce(json_extract(published_seo,'$.canonical'),'')='' OR json_extract(published_seo,'$.canonical')=(SELECT base_url FROM installation WHERE id=1)||'/'||published_slug)"
	args := []any{}
	if len(options) > 0 {
		o := options[0]
		if o.NoIndexPages || o.ExcludeSitemapPages {
			where += " AND kind<>'page'"
		}
		if o.NoIndexPosts || o.ExcludeSitemapPosts {
			where += " AND kind<>'post'"
		}
		for _, line := range strings.Split(o.ExcludeSitemapPaths, "\n") {
			if value := strings.TrimSpace(line); value != "" {
				where += " AND published_slug<>?"
				args = append(args, strings.TrimPrefix(value, "/"))
			}
		}
	}
	return where, args
}
func (s *Store) SitemapCount(options ...SEOOptions) (int, error) {
	var total int
	where, args := sitemapFilter(options)
	err := s.DB.QueryRow("SELECT count(*) FROM pages WHERE "+where, args...).Scan(&total)
	return total, err
}
func (s *Store) SitemapPages(offset, limit int, options ...SEOOptions) ([]Page, error) {
	where, args := sitemapFilter(options)
	args = append(args, limit, offset)
	rows, err := s.DB.Query("SELECT published_slug,published_updated_at FROM pages WHERE "+where+" ORDER BY id LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []Page{}
	for rows.Next() {
		var p Page
		if err = rows.Scan(&p.Slug, &p.PublishedUpdatedAt); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
func (s *Store) FeedPosts() ([]Page, error) {
	rows, err := s.DB.Query("SELECT id,published_slug,published_title,published_document,published_seo,published_at FROM pages WHERE kind='post' AND " + publicIndexWhere + " ORDER BY published_at DESC,id DESC LIMIT 30")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []Page{}
	for rows.Next() {
		var p Page
		var seoJSON string
		if err = rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Document, &seoJSON, &p.PublishedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(seoJSON), &p.SEO); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
