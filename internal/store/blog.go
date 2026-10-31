package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

var ErrInvalidTerms = errors.New("invalid content taxonomy")

type Term struct {
	ID         int64
	Kind, Name string
	Version    int64
}

func (s *Store) Terms(public bool) ([]Term, error) {
	query := "SELECT id,kind,name,version FROM terms WHERE deleted_at=0"
	if public {
		query += " AND id IN(SELECT DISTINCT value FROM pages,json_each(pages.published_term_ids) WHERE pages.kind='post' AND pages.trashed_at=0 AND pages.published_slug IS NOT NULL)"
	}
	rows, err := s.DB.Query(query + " ORDER BY kind,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Term{}
	for rows.Next() {
		var t Term
		if err = rows.Scan(&t.ID, &t.Kind, &t.Name, &t.Version); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (s *Store) SaveTerm(ctx context.Context, t Term, remove bool) error {
	if t.ID == 0 {
		_, err := s.DB.ExecContext(ctx, "INSERT INTO terms(kind,name,name_key) VALUES(?,?,?)", t.Kind, t.Name, strings.ToLower(t.Name))
		return err
	}
	query := "UPDATE terms SET name=?,name_key=?,version=version+1 WHERE id=? AND version=? AND deleted_at=0"
	value := any(t.Name)
	if remove {
		query = "UPDATE terms SET deleted_at=?,version=version+1 WHERE id=? AND version=? AND deleted_at=0"
		value = 1
	}
	args := []any{value, strings.ToLower(t.Name), t.ID, t.Version}
	if remove {
		args = []any{value, t.ID, t.Version}
	}
	res, err := s.DB.ExecContext(ctx, query, args...)
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
	return nil
}
func (s *Store) ActiveTermIDs(ids []int64) ([]int64, error) {
	raw, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	var active string
	err = s.DB.QueryRow("SELECT json_group_array(id) FROM terms WHERE deleted_at=0 AND id IN(SELECT value FROM json_each(?))", string(raw)).Scan(&active)
	if err != nil {
		return nil, err
	}
	result := []int64{}
	err = json.Unmarshal([]byte(active), &result)
	return result, err
}
func (s *Store) Schedule(ctx context.Context, id, version, at int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "UPDATE pages SET scheduled_at=?,scheduled_version=version+1,version=version+1,updated_at=unixepoch() WHERE id=? AND version=? AND trashed_at=0", at, id, version)
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
	if _, err = tx.ExecContext(ctx, revisionSQL, id); err != nil {
		return err
	}
	return tx.Commit()
}

// PublishDue publishes pinned snapshots, preserving any later working draft.
func (s *Store) PublishDue(ctx context.Context, now int64) (int, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id FROM pages WHERE scheduled_at>0 AND scheduled_at<=? AND trashed_at=0 ORDER BY scheduled_at,id LIMIT 10", now)
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for i, id := range ids {
		if err = s.publishDuePage(ctx, id, now); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}

func (s *Store) publishDuePage(ctx context.Context, id, now int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var due bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pages WHERE id=? AND scheduled_at>0 AND scheduled_at<=? AND trashed_at=0)", id, now).Scan(&due); err != nil {
		return err
	}
	if !due {
		return nil
	}
	if _, err = tx.ExecContext(ctx, "SAVEPOINT publication"); err != nil {
		return err
	}
	err = publishScheduled(ctx, tx, id, now)
	if errors.Is(err, ErrConflict) || errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.ExecContext(ctx, "ROLLBACK TO publication"); err != nil {
			return err
		}
		var p Page
		p.ID = id
		if err = tx.QueryRowContext(ctx, "SELECT title,kind,version FROM pages WHERE id=?", id).Scan(&p.Title, &p.Kind, &p.Version); err != nil {
			return err
		}
		// Keep the draft and pinned revision, but remove a job that cannot publish safely.
		if err = changeContent(ctx, tx, id, p.Version, "cancel_schedule"); err != nil {
			return err
		}
		p.Version++
		if err = auditContent(ctx, tx, "schedule_failed", p); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PublicPosts(term int64, page int, sizes ...int) ([]Page, int, error) {
	size := 12
	if len(sizes) > 0 {
		size = sizes[0]
	}
	return s.PublicPostsLanguage(term, page, size, "")
}
func (s *Store) PublicPostsLanguage(term int64, page, size int, language string) ([]Page, int, error) {
	if size < 1 || size > 100 {
		size = 12
	}
	if page < 1 {
		page = 1
	}
	where := "p.kind='post' AND trashed_at=0 AND published_slug IS NOT NULL"
	args := []any{}
	if term > 0 {
		where += " AND EXISTS(SELECT 1 FROM json_each(published_term_ids) WHERE value=?) AND EXISTS(SELECT 1 FROM terms WHERE id=? AND deleted_at=0)"
		args = append(args, term, term)
	}
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM pages p WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(`SELECT p.id,p.published_slug,coalesce(json_extract(t.value,'$.title'),p.published_title),coalesce(p.published_cover,''),p.published_at,coalesce(json_extract(t.value,'$.seo'),p.published_seo),p.published_term_ids,t.key FROM pages p LEFT JOIN json_each(p.published_translations) t ON t.key=? WHERE `+where+" ORDER BY p.published_at DESC,p.id DESC LIMIT ? OFFSET ?", append([]any{language}, append(args, size, (page-1)*size)...)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	posts := []Page{}
	for rows.Next() {
		var p Page
		var seo, terms string
		var translated sql.NullString
		if err = rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Cover, &p.PublishedAt, &seo, &terms, &translated); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(seo), &p.SEO); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(terms), &p.TermIDs); err != nil {
			return nil, 0, err
		}
		p.Kind = "post"
		p.URL = "/" + p.Slug
		if translated.Valid {
			p.URL += "?lang=" + url.QueryEscape(translated.String)
		}
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}
