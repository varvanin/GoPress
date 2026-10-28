package store

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"strconv"
	"strings"
)

func reserveRoute(ctx context.Context, tx *sql.Tx, slug string, id int64) error {
	if _, err := tx.ExecContext(ctx, "INSERT INTO routes(slug,page_id) VALUES(?,?) ON CONFLICT(slug) DO NOTHING", slug, id); err != nil {
		return err
	}
	var owner int64
	if err := tx.QueryRowContext(ctx, "SELECT page_id FROM routes WHERE slug=?", slug).Scan(&owner); err != nil {
		return err
	}
	if owner != id {
		return ErrConflict
	}
	return nil
}
func pagePath(ctx context.Context, tx *sql.Tx, id, parent int64, slug, kind string) (string, error) {
	if kind != "page" {
		if parent != 0 || strings.Contains(slug, "/") {
			return "", ErrConflict
		}
		return slug, nil
	}
	if parent == 0 {
		return path.Base(slug), nil
	}
	var parentSlug string
	if err := tx.QueryRowContext(ctx, "SELECT published_slug FROM pages WHERE id=? AND kind='page' AND trashed_at=0 AND published_slug IS NOT NULL", parent).Scan(&parentSlug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrConflict
		}
		return "", err
	}
	var cycle, depth int
	if err := tx.QueryRowContext(ctx, `WITH RECURSIVE ancestors(id) AS (VALUES(?) UNION SELECT p.parent_id FROM pages p JOIN ancestors a ON p.id=a.id WHERE p.parent_id>0 UNION SELECT p.published_parent_id FROM pages p JOIN ancestors a ON p.id=a.id WHERE p.published_parent_id>0) SELECT coalesce(sum(id=?),0),count(*) FROM ancestors`, parent, id).Scan(&cycle, &depth); err != nil {
		return "", err
	}
	if cycle > 0 || depth >= 32 {
		return "", ErrConflict
	}
	result := parentSlug + "/" + path.Base(slug)
	if len(result) > 1024 {
		return "", ErrConflict
	}
	return result, nil
}
func publishCurrent(ctx context.Context, tx *sql.Tx, id, version int64) error {
	var old, slug, kind string
	var parent int64
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(published_slug,''),slug,kind,parent_id FROM pages WHERE id=? AND version=? AND trashed_at=0", id, version).Scan(&old, &slug, &kind, &parent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		return err
	}
	target, err := pagePath(ctx, tx, id, parent, slug, kind)
	if err != nil {
		return err
	}
	if err = reserveRoute(ctx, tx, target, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE pages SET slug=? WHERE id=?", target, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, publishSQL, id, version)
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
	return movePublishedChildren(ctx, tx, id, old, target)
}
func movePublishedChildren(ctx context.Context, tx *sql.Tx, id int64, old, target string) error {
	if old == "" || old == target {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE routes SET redirect=1 WHERE slug=? AND page_id=?", old, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE routes SET redirect=0 WHERE slug=? AND page_id=?", target, id); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `WITH RECURSIVE children(id) AS (SELECT id FROM pages WHERE published_parent_id=? AND published_slug IS NOT NULL AND trashed_at=0 UNION SELECT p.id FROM pages p JOIN children c ON p.published_parent_id=c.id WHERE p.published_slug IS NOT NULL AND p.trashed_at=0) SELECT id,slug,published_slug,parent_id,published_parent_id FROM pages WHERE id IN(SELECT id FROM children) ORDER BY length(published_slug),id`, id)
	if err != nil {
		return err
	}
	type child struct {
		id, parent, publishedParent int64
		draft, published            string
	}
	children := []child{}
	for rows.Next() {
		var c child
		if err = rows.Scan(&c.id, &c.draft, &c.published, &c.parent, &c.publishedParent); err != nil {
			rows.Close()
			return err
		}
		children = append(children, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range children {
		if !strings.HasPrefix(c.published, old+"/") {
			return ErrConflict
		}
		next := target + strings.TrimPrefix(c.published, old)
		if len(next) > 1024 {
			return ErrConflict
		}
		if err = reserveRoute(ctx, tx, next, c.id); err != nil {
			return err
		}
		draft := c.draft
		if c.parent == c.publishedParent && strings.HasPrefix(draft, old+"/") {
			draft = target + strings.TrimPrefix(draft, old)
			if err = reserveRoute(ctx, tx, draft, c.id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE pages SET slug=?,published_slug=?,published_updated_at=unixepoch(),updated_at=unixepoch(),version=version+1 WHERE id=?", draft, next, c.id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE routes SET redirect=1 WHERE slug=? AND page_id=?", c.published, c.id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE routes SET redirect=0 WHERE slug=? AND page_id=?", next, c.id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, revisionSQL, c.id); err != nil {
			return err
		}
	}
	return auditTx(ctx, tx, "content.move", strconv.FormatInt(id, 10))
}
func (s *Store) Redirect(slug string) (string, error) {
	var target string
	err := s.DB.QueryRow("SELECT p.published_slug FROM routes r JOIN pages p ON p.id=r.page_id WHERE r.slug=? AND r.redirect=1 AND p.published_slug IS NOT NULL AND p.trashed_at=0", slug).Scan(&target)
	return target, err
}

func publishScheduled(ctx context.Context, tx *sql.Tx, id, now int64) error {
	var old, kind, slug string
	var revision, parent int64
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(published_slug,''),kind,scheduled_version FROM pages WHERE id=? AND trashed_at=0", id).Scan(&old, &kind, &revision); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT slug,parent_id FROM revisions WHERE page_id=? AND version=?", id, revision).Scan(&slug, &parent); err != nil {
		return err
	}
	target, err := pagePath(ctx, tx, id, parent, slug, kind)
	if err != nil {
		return err
	}
	if err = reserveRoute(ctx, tx, target, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE pages SET
 (published_title,published_document,published_cover,published_term_ids,published_seo,published_parent_id,published_sort_order,published_template)=
 (SELECT title,document,cover,term_ids,seo,parent_id,sort_order,template FROM revisions WHERE page_id=pages.id AND version=pages.scheduled_version),
 published_slug=?,published_at=CASE WHEN published_slug IS NULL THEN ? ELSE published_at END,published_updated_at=?,scheduled_at=0,scheduled_version=0,updated_at=?,version=version+1 WHERE id=?`, target, now, now, now, id)
	if err != nil {
		return err
	}
	if err = movePublishedChildren(ctx, tx, id, old, target); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, revisionSQL, id); err != nil {
		return err
	}
	var title string
	if err = tx.QueryRowContext(ctx, "SELECT published_title FROM pages WHERE id=?", id).Scan(&title); err != nil {
		return err
	}
	return auditContent(ctx, tx, "scheduled", Page{ID: id, Title: title, Kind: kind})
}
