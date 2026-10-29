package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

type MediaFile struct {
	Name, Title, Alt, Caption, Revision string
	Size, CreatedAt                     int64
}

// Include immutable revisions: restoring an old version must never resurrect a broken image.
const mediaUsed = `EXISTS(SELECT 1 FROM module_collection_media WHERE media_name=m.name) OR EXISTS(SELECT 1 FROM pages WHERE cover=m.name OR published_cover=m.name
 OR EXISTS(SELECT 1 FROM json_tree(document) WHERE key='src' AND value='/uploads/'||m.name)
 OR EXISTS(SELECT 1 FROM json_tree(coalesce(published_document,'{}')) WHERE key='src' AND value='/uploads/'||m.name))
 OR EXISTS(SELECT 1 FROM revisions WHERE cover=m.name
 OR EXISTS(SELECT 1 FROM json_tree(document) WHERE key='src' AND value='/uploads/'||m.name))
 OR EXISTS(SELECT 1 FROM pages p,json_tree(p.seo) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM pages p,json_tree(coalesce(p.published_seo,'{}')) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM revisions r,json_tree(r.seo) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM pages p,json_each(p.translations) t,json_tree(json_extract(t.value,'$.document')) j WHERE j.key='src' AND j.value='/uploads/'||m.name)
 OR EXISTS(SELECT 1 FROM pages p,json_each(p.published_translations) t,json_tree(json_extract(t.value,'$.document')) j WHERE j.key='src' AND j.value='/uploads/'||m.name)
 OR EXISTS(SELECT 1 FROM revisions r,json_each(r.translations) t,json_tree(json_extract(t.value,'$.document')) j WHERE j.key='src' AND j.value='/uploads/'||m.name)
 OR EXISTS(SELECT 1 FROM pages p,json_tree(p.translations) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM pages p,json_tree(p.published_translations) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM revisions r,json_tree(r.translations) j WHERE j.key='social_image' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM appearance a,json_tree(a.value) j WHERE j.key IN('DefaultImage','LogoURL') AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)
 OR EXISTS(SELECT 1 FROM module_packages p,json_tree(p.settings) j WHERE j.type='text' AND gopress_upload_name(j.value,(SELECT base_url FROM installation WHERE id=1))=m.name)`

func (s *Store) ListMedia(query string, page int, owners ...int64) ([]MediaFile, int, error) {
	if page < 1 {
		page = 1
	}
	pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(query)) + "%"
	owner := int64(0)
	if len(owners) > 0 {
		owner = owners[0]
	}
	const where = ` WHERE gopress_lower(m.name||' '||coalesce(d.title,'')||' '||coalesce(d.alt,'')||' '||coalesce(d.caption,'')) LIKE ? ESCAPE '\' AND (?=0 OR m.owner_id=?)`
	var total int
	if err := s.DB.QueryRow(`SELECT count(*) FROM media m LEFT JOIN media_details d ON d.name=m.name`+where, pattern, owner, owner).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(`SELECT m.name,coalesce(d.title,''),coalesce(d.alt,''),coalesce(d.caption,''),m.size,coalesce(d.created_at,0) FROM media m LEFT JOIN media_details d ON d.name=m.name`+where+` ORDER BY coalesce(d.created_at,0) DESC,m.rowid DESC LIMIT 24 OFFSET ?`, pattern, owner, owner, (page-1)*24)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	files := []MediaFile{}
	for rows.Next() {
		var f MediaFile
		if err = rows.Scan(&f.Name, &f.Title, &f.Alt, &f.Caption, &f.Size, &f.CreatedAt); err != nil {
			return nil, 0, err
		}
		f.Revision = mediaRevision(f)
		files = append(files, f)
	}
	return files, total, rows.Err()
}
func (s *Store) MediaFile(name string) (MediaFile, error) {
	var f MediaFile
	err := s.DB.QueryRow(`SELECT m.name,coalesce(d.title,''),coalesce(d.alt,''),coalesce(d.caption,''),m.size,coalesce(d.created_at,0) FROM media m LEFT JOIN media_details d ON d.name=m.name WHERE m.name=?`, name).Scan(&f.Name, &f.Title, &f.Alt, &f.Caption, &f.Size, &f.CreatedAt)
	f.Revision = mediaRevision(f)
	return f, err
}
func (s *Store) MediaFiles(names []string) (map[string]MediaFile, error) {
	result := map[string]MediaFile{}
	if len(names) == 0 {
		return result, nil
	}
	raw, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT m.name,coalesce(d.title,''),coalesce(d.alt,''),coalesce(d.caption,''),m.size,coalesce(d.created_at,0) FROM media m LEFT JOIN media_details d ON d.name=m.name WHERE m.name IN(SELECT value FROM json_each(?))`, string(raw))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f MediaFile
		if err = rows.Scan(&f.Name, &f.Title, &f.Alt, &f.Caption, &f.Size, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Revision = mediaRevision(f)
		result[f.Name] = f
	}
	return result, rows.Err()
}
func mediaRevision(f MediaFile) string {
	raw, _ := json.Marshal([]string{f.Name, f.Title, f.Alt, f.Caption})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s *Store) SaveMediaDetails(f MediaFile) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current := MediaFile{Name: f.Name}
	if err = tx.QueryRow("SELECT coalesce(d.title,''),coalesce(d.alt,''),coalesce(d.caption,'') FROM media m LEFT JOIN media_details d ON d.name=m.name WHERE m.name=?", f.Name).Scan(&current.Title, &current.Alt, &current.Caption); err != nil {
		return err
	}
	if f.Revision != mediaRevision(current) {
		return ErrConflict
	}
	_, err = tx.Exec(`INSERT INTO media_details(name,title,alt,caption) VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET title=excluded.title,alt=excluded.alt,caption=excluded.caption`, f.Name, f.Title, f.Alt, f.Caption)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteMedia(ctx context.Context, name string) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM media AS m WHERE name=? AND NOT (`+mediaUsed+`)`, name)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}

type MenuItem struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Depth int    `json:"depth"`
}
type Menu struct {
	Position string
	Version  int64
	Items    []MenuItem
}

func (s *Store) Menu(position string) (Menu, error) {
	m := Menu{Position: position}
	var raw string
	err := s.DB.QueryRow(`SELECT version,items FROM menus WHERE position=?`, position).Scan(&m.Version, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &m.Items)
	}
	return m, err
}
func (s *Store) SaveMenu(ctx context.Context, m Menu) error {
	raw, err := json.Marshal(m.Items)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE menus SET version=version+1,items=? WHERE position=? AND version=?`, string(raw), m.Position, m.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}

var ErrMediaUsed = errors.New("media is used by content or revisions")

func (s *Store) MediaUsed(name string) (bool, error) {
	var used bool
	err := s.DB.QueryRow("SELECT ("+mediaUsed+") FROM media m WHERE name=?", name).Scan(&used)
	return used, err
}

func referencedUpload(value, base string) string {
	u, err := url.Parse(value)
	if err != nil || u.User != nil {
		return ""
	}
	if u.Host != "" || u.Scheme != "" {
		origin, e := url.Parse(base)
		if e != nil || !strings.EqualFold(u.Host, origin.Host) || !strings.EqualFold(u.Scheme, origin.Scheme) {
			return ""
		}
	}
	if !strings.HasPrefix(u.Path, "/uploads/") {
		return ""
	}
	name := strings.TrimPrefix(u.Path, "/uploads/")
	if name == "" || name == "." || name == ".." || path.Base(name) != name || strings.Contains(name, "\\") {
		return ""
	}
	return name
}
