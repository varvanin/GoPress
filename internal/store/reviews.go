package store

import (
	"context"
	"database/sql"
	"strconv"
)

type ContentReview struct {
	PageID, Version, RequestedBy, RequestedAt int64
	Title, Kind, Author, Comment              string
	Current                                   bool
}

func (s *Store) RequestReview(ctx context.Context, id, version int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO content_reviews(page_id,version,requested_by) SELECT id,version,? FROM pages WHERE id=? AND version=? AND trashed_at=0 ON CONFLICT(page_id) DO UPDATE SET version=excluded.version,requested_by=excluded.requested_by,requested_at=unixepoch(),comment=''`, Owner(ctx), id, version)
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
	if err = auditTx(ctx, tx, "content.review_requested", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

// shortcut: Show 100 reviews, actionable ones first; add pagination when a site needs a larger queue.
func (s *Store) ContentReviews(owner int64) ([]ContentReview, error) {
	rows, err := s.DB.Query(`SELECT r.page_id,r.version,r.requested_by,r.requested_at,p.title,p.kind,u.name,r.comment,p.version=r.version FROM content_reviews r JOIN pages p ON p.id=r.page_id JOIN users u ON u.id=r.requested_by WHERE p.trashed_at=0 AND (?=0 OR p.owner_id=?) ORDER BY (r.comment='' AND p.version=r.version) DESC,r.requested_at,r.page_id LIMIT 100`, owner, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ContentReview{}
	for rows.Next() {
		var r ContentReview
		if err = rows.Scan(&r.PageID, &r.Version, &r.RequestedBy, &r.RequestedAt, &r.Title, &r.Kind, &r.Author, &r.Comment, &r.Current); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (s *Store) ResolveReview(ctx context.Context, id, version int64, publish bool, comment string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var submitted int64
	var previousComment string
	if err = tx.QueryRowContext(ctx, `SELECT version,comment FROM content_reviews WHERE page_id=?`, id).Scan(&submitted, &previousComment); err != nil {
		if err == sql.ErrNoRows {
			return ErrConflict
		}
		return err
	}
	if submitted != version || previousComment != "" {
		return ErrConflict
	}
	if publish {
		if err = changeContent(ctx, tx, id, version, "publish"); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM content_reviews WHERE page_id=?`, id); err != nil {
			return err
		}
	} else {
		// A return keeps the review note visible; resubmission explicitly clears it.
		if _, err = tx.ExecContext(ctx, `UPDATE content_reviews SET comment=? WHERE page_id=?`, comment, id); err != nil {
			return err
		}
	}
	if err = auditTx(ctx, tx, "content.review_resolved", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
