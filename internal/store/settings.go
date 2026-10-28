package store

import (
	"context"
	"encoding/json"
)

type Settings struct {
	FormRetentionDays  int    `json:"form_retention_days"`
	AuditRetentionDays int    `json:"audit_retention_days"`
	Version            int64  `json:"-"`
	Site               Site   `json:"-"`
	Description        string `json:"description"`
	HomeMode           string `json:"home_mode"`
	HomePageID         int64  `json:"home_page_id"`
	PostsPerPage       int    `json:"posts_per_page"`
	Language           string `json:"language"`
}

func (s *Store) Settings() (Settings, error) {
	var value Settings
	var raw string
	err := s.DB.QueryRow("SELECT s.version,s.value,i.title,i.base_url,i.timezone FROM settings s JOIN installation i ON i.id=s.id WHERE s.id=1").Scan(&value.Version, &raw, &value.Site.Title, &value.Site.BaseURL, &value.Site.Timezone)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &value)
	}
	return value, err
}
func (s *Store) SaveSettings(ctx context.Context, value Settings) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldMode string
	var oldPage int64
	if err = tx.QueryRowContext(ctx, "SELECT json_extract(value,'$.home_mode'),coalesce(json_extract(value,'$.home_page_id'),0) FROM settings WHERE id=1").Scan(&oldMode, &oldPage); err != nil {
		return err
	}
	if value.HomeMode == "page" && (oldMode != "page" || oldPage != value.HomePageID) {
		var n int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM pages WHERE id=? AND kind='page' AND trashed_at=0 AND published_slug IS NOT NULL", value.HomePageID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
	}
	res, err := tx.ExecContext(ctx, "UPDATE settings SET value=?,version=version+1 WHERE id=1 AND version=?", string(raw), value.Version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "UPDATE installation SET title=?,base_url=?,timezone=? WHERE id=1", value.Site.Title, value.Site.BaseURL, value.Site.Timezone); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) PublicPage(id int64) (Page, error) {
	var slug string
	if err := s.DB.QueryRow("SELECT published_slug FROM pages WHERE id=? AND kind='page' AND trashed_at=0 AND published_slug IS NOT NULL", id).Scan(&slug); err != nil {
		return Page{}, err
	}
	return s.Public(slug)
}
func (s *Store) HomePages() ([]Page, error) {
	rows, err := s.DB.Query("SELECT id,published_title FROM pages WHERE kind='page' AND trashed_at=0 AND published_slug IS NOT NULL ORDER BY published_title,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []Page{}
	for rows.Next() {
		var p Page
		if err = rows.Scan(&p.ID, &p.Title); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
