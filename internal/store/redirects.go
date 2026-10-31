package store

import (
	"context"
	"strconv"
)

type ManualRedirect struct {
	Slug, Target, Title string
	PageID              int64
}
type MissingPath struct {
	Slug           string
	Hits, LastSeen int64
}

func (s *Store) ManualRedirects() ([]ManualRedirect, error) {
	rows, err := s.DB.Query(`SELECT r.slug,r.page_id,coalesce(p.published_slug,''),p.title FROM manual_redirects r JOIN pages p ON p.id=r.page_id ORDER BY r.slug LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ManualRedirect{}
	for rows.Next() {
		var r ManualRedirect
		if err = rows.Scan(&r.Slug, &r.PageID, &r.Target, &r.Title); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (s *Store) SaveManualRedirect(ctx context.Context, slug string, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE id=? AND published_slug IS NOT NULL AND trashed_at=0 AND published_slug<>?) AND NOT EXISTS(SELECT 1 FROM routes WHERE slug=?) AND (SELECT count(*) FROM manual_redirects)<500`, id, slug, slug).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrConflict
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO manual_redirects(slug,page_id) VALUES(?,?) ON CONFLICT(slug) DO NOTHING`, slug, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM missing_paths WHERE slug=?`, slug); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "redirect.create", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DeleteManualRedirect(ctx context.Context, slug string, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM manual_redirects WHERE slug=? AND page_id=?`, slug, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if err = auditTx(ctx, tx, "redirect.delete", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ManualRedirect(slug string) (string, error) {
	var target string
	err := s.DB.QueryRow(`SELECT p.published_slug FROM manual_redirects r JOIN pages p ON p.id=r.page_id WHERE r.slug=? AND p.published_slug IS NOT NULL AND p.trashed_at=0`, slug).Scan(&target)
	return target, err
}
func (s *Store) RecordMissingPath(ctx context.Context, slug string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO missing_paths(slug) SELECT ? WHERE EXISTS(SELECT 1 FROM missing_paths WHERE slug=?) OR (SELECT count(*) FROM missing_paths)<500 ON CONFLICT(slug) DO UPDATE SET hits=min(hits+1,2147483647),last_seen=unixepoch()`, slug, slug)
	return err
}
func (s *Store) MissingPaths() ([]MissingPath, error) {
	rows, err := s.DB.Query(`SELECT slug,hits,last_seen FROM missing_paths ORDER BY hits DESC,last_seen DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MissingPath{}
	for rows.Next() {
		var p MissingPath
		if err = rows.Scan(&p.Slug, &p.Hits, &p.LastSeen); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (s *Store) ClearMissingPaths(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM missing_paths`)
	return err
}
func (s *Store) RedirectTargets() ([]Page, error) {
	rows, err := s.DB.Query(`SELECT id,published_slug,published_title FROM pages WHERE trashed_at=0 AND published_slug IS NOT NULL ORDER BY published_title,id LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Page{}
	for rows.Next() {
		var p Page
		if err = rows.Scan(&p.ID, &p.Slug, &p.Title); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
