package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
)

// SQLite's built-in lower() only handles ASCII; admin search must also match Cyrillic.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("gopress_upload_name", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, _ := args[0].(string)
		base, _ := args[1].(string)
		return referencedUpload(value, base), nil
	})
	sqlite.MustRegisterDeterministicScalarFunction("gopress_lower", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		value, _ := args[0].(string)
		return strings.ToLower(value), nil
	})
}

//go:embed migration.sql
var migration string

//go:embed content.sql
var contentMigration string

//go:embed blog.sql
var blogMigration string

//go:embed site.sql
var siteMigration string

//go:embed seo.sql
var seoMigration string

//go:embed forms.sql
var formsMigration string

//go:embed settings.sql
var settingsMigration string

//go:embed users.sql
var usersMigration string

//go:embed search.sql
var searchMigration string

//go:embed pages.sql
var pagesMigration string

//go:embed jobs.sql
var jobsMigration string

//go:embed faq.sql
var faqMigration string

//go:embed module_packages.sql
var modulePackagesMigration string

var ErrConflict = errors.New("version or URL conflict")
var ErrInstalled = errors.New("already installed")

type Store struct {
	DB   *sql.DB
	Dir  string
	lock *os.File
}
type Site struct {
	Title    string
	BaseURL  string
	Timezone string
}
type SEO struct {
	Canonical         string `json:"canonical,omitempty"`
	SocialTitle       string `json:"social_title,omitempty"`
	SocialDescription string `json:"social_description,omitempty"`
	SocialImage       string `json:"social_image,omitempty"`
	NoFollow          bool   `json:"nofollow,omitempty"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	NoIndex           bool   `json:"noindex"`
}

type Page struct {
	ParentID           int64   `json:"parent_id"`
	Order              int     `json:"order"`
	Template           string  `json:"template"`
	PublishedParentID  int64   `json:"published_parent_id"`
	OwnerID            int64   `json:"owner_id"`
	SEO                SEO     `json:"seo"`
	PublishedUpdatedAt int64   `json:"published_updated_at"`
	TermIDs            []int64 `json:"term_ids"`
	ScheduledAt        int64   `json:"scheduled_at"`
	ScheduledVersion   int64   `json:"scheduled_version"`
	CreatedAt          int64   `json:"created_at"`
	UpdatedAt          int64   `json:"updated_at"`
	PublishedAt        int64   `json:"published_at"`
	TrashedAt          int64   `json:"trashed_at"`
	Kind               string  `json:"kind"`
	ID                 int64   `json:"id"`
	Slug               string  `json:"slug"`
	Version            int64   `json:"version"`
	Title              string  `json:"title"`
	Document           string  `json:"document"`
	Cover              string  `json:"cover"`
	Published          bool    `json:"published"`
	PublishedSlug      string  `json:"published_slug"`
}

// Open holds the data-directory lock until Close, including during maintenance.
func Open(dir string) (*Store, error) { return openWithLock(dir, nil) }
func openWithLock(dir string, held *os.File) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	f := held
	if f == nil {
		f, err = os.OpenFile(filepath.Join(abs, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			f.Close()
			return nil, fmt.Errorf("data directory is locked: %w", err)
		}
	}
	s := &Store{Dir: abs, lock: f}
	ok := false
	defer func() {
		if !ok {
			if held != nil {
				if s.DB != nil {
					s.DB.Close()
				}
			} else {
				s.Close()
			}
		}
	}()
	if held == nil {
		if _, err := os.Stat(filepath.Join(abs, "restore-in-progress")); err == nil {
			return nil, errors.New("incomplete restore: keep maintenance enabled and run backup recover --source VERIFIED_BACKUP --data DATA")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	dbpath := filepath.Join(abs, "site.db")
	_, statErr := os.Stat(dbpath)
	fresh := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !fresh {
		return nil, statErr
	}
	if fresh {
		file, e := os.OpenFile(dbpath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		if e = file.Close(); e != nil {
			return nil, e
		}
	}
	u := url.URL{Scheme: "file", Path: dbpath}
	q := url.Values{}
	q.Set("mode", "rw")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = q.Encode()
	s.DB, err = sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	// One connection serializes all SQL, with bounded waits and connection-wide DSN pragmas.
	s.DB.SetMaxOpenConns(1)
	s.DB.SetMaxIdleConns(1)
	var version int
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, fmt.Errorf("read database: %w", err)
	}
	if version < 0 || version > 14 || (!fresh && version == 0) {
		return nil, fmt.Errorf("unsupported database schema %d; refusing to reinstall", version)
	}
	if version < 14 {
		tx, e := s.DB.Begin()
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		scripts := []string{migration, "ALTER TABLE pages ADD COLUMN kind TEXT NOT NULL DEFAULT 'page' CHECK(kind IN ('page','post')); PRAGMA user_version=2;", contentMigration, blogMigration, siteMigration, seoMigration, formsMigration, settingsMigration, usersMigration, searchMigration, pagesMigration, jobsMigration, faqMigration, modulePackagesMigration}
		for _, script := range scripts[version:] {
			if _, e = tx.Exec(script); e != nil {
				return nil, e
			}
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
	}
	if err = s.Check(); err != nil {
		return nil, err
	}
	var mode string
	if err = s.DB.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return nil, err
	}
	if mode != "wal" {
		return nil, errors.New("WAL unavailable")
	}
	if err = os.MkdirAll(filepath.Join(abs, "uploads"), 0700); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}
func (s *Store) Close() error {
	var err error
	if s.DB != nil {
		err = s.DB.Close()
	}
	if s.lock != nil {
		_ = unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
		err = errors.Join(err, s.lock.Close())
		s.lock = nil
	}
	return err
}
func (s *Store) Check() error {
	var result string
	if err := s.DB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("database integrity check failed")
	}
	rows, err := s.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key check failed")
	}
	return rows.Err()
}
func (s *Store) Site() (Site, error) {
	var v Site
	err := s.DB.QueryRow("SELECT title,base_url,timezone FROM installation WHERE id=1").Scan(&v.Title, &v.BaseURL, &v.Timezone)
	return v, err
}
func (s *Store) Install(ctx context.Context, site Site, email string, hash []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM installation").Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return ErrInstalled
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,email,password_hash) VALUES(1,?,?)", email, hash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO installation VALUES(1,?,?,?,unixepoch())", site.Title, site.BaseURL, site.Timezone); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Password(email string) ([]byte, error) {
	var h []byte
	err := s.DB.QueryRow("SELECT password_hash FROM users WHERE email=? AND blocked=0", email).Scan(&h)
	return h, err
}
func (s *Store) Session(hash string, users ...int64) error {
	userID := int64(1)
	if len(users) > 0 {
		userID = users[0]
	}
	_, err := s.DB.Exec("DELETE FROM sessions WHERE expires_at<=unixepoch()")
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO sessions(hash,expires_at,user_id) VALUES(?,unixepoch()+43200,?)", hash, userID)
	return err
}
func (s *Store) Authorized(hash string) (bool, error) {
	var n int
	err := s.DB.QueryRow("SELECT count(*) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=? AND s.expires_at>unixepoch() AND u.blocked=0", hash).Scan(&n)
	return n == 1, err
}
func (s *Store) Logout(hash string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE hash=?", hash)
	return err
}
func (s *Store) RevokeSessions() error {
	_, err := s.DB.Exec("DELETE FROM sessions; DELETE FROM password_resets")
	return err
}
func (s *Store) Media(name, mime string, size int64, owners ...int64) error {
	owner := int64(1)
	if len(owners) > 0 {
		owner = owners[0]
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO media(name,mime,size,owner_id) VALUES(?,?,?,?)", name, mime, size, owner); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO media_details(name) VALUES(?)", name); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) MediaType(name string) (string, error) {
	var t string
	err := s.DB.QueryRow("SELECT mime FROM media WHERE name=?", name).Scan(&t)
	return t, err
}
func (s *Store) MediaNames(owners ...int64) ([]string, error) {
	owner := int64(0)
	if len(owners) > 0 {
		owner = owners[0]
	}
	rows, err := s.DB.Query("SELECT name FROM media WHERE (?=0 OR owner_id=?) ORDER BY name LIMIT 100", owner, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

const pageColumns = "id,slug,version,title,document,coalesce(cover,''),published_slug IS NOT NULL,coalesce(published_slug,''),kind,created_at,updated_at,published_at,trashed_at,term_ids,scheduled_at,scheduled_version,seo,published_updated_at,owner_id,parent_id,sort_order,template,published_parent_id"

func scanPage(row interface{ Scan(...any) error }) (Page, error) {
	var p Page
	var termJSON, seoJSON string
	err := row.Scan(&p.ID, &p.Slug, &p.Version, &p.Title, &p.Document, &p.Cover, &p.Published, &p.PublishedSlug, &p.Kind, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &p.TrashedAt, &termJSON, &p.ScheduledAt, &p.ScheduledVersion, &seoJSON, &p.PublishedUpdatedAt, &p.OwnerID, &p.ParentID, &p.Order, &p.Template, &p.PublishedParentID)
	if err == nil {
		err = json.Unmarshal([]byte(termJSON), &p.TermIDs)
		if err == nil {
			err = json.Unmarshal([]byte(seoJSON), &p.SEO)
		}
	}
	return p, err
}

const ContentPageSize = 25

// ListContent reads metadata only: large editor documents never enter list responses.
func (s *Store) ListContent(kind, query string, trash bool, limit, offset int, termIDs ...int64) ([]Page, int, error) {
	if (kind != "page" && kind != "post") || limit < 1 || limit > 100 || offset < 0 || len(query) > 200 {
		return nil, 0, errors.New("invalid content query")
	}
	where := "kind=? AND trashed_at=0"
	if trash {
		where = "kind=? AND trashed_at>0"
	}
	args := []any{kind}
	if query != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(query))
		where += ` AND (gopress_lower(title) LIKE ? ESCAPE '\' OR slug LIKE ? ESCAPE '\')`
		args = append(args, "%"+escaped+"%", "%"+escaped+"%")
	}
	if len(termIDs) > 1 && termIDs[1] > 0 {
		where += " AND owner_id=?"
		args = append(args, termIDs[1])
	}
	if len(termIDs) > 0 && termIDs[0] > 0 {
		where += " AND EXISTS(SELECT 1 FROM json_each(pages.term_ids) WHERE value=?)"
		args = append(args, termIDs[0])
	}
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM pages WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	pages, err := s.contentRows(where, args, limit, offset)
	return pages, total, err
}
func (s *Store) contentRows(where string, args []any, limit, offset int) ([]Page, error) {
	columns := strings.Replace(pageColumns, ",document,", ",'' AS document,", 1)
	rows, err := s.DB.Query("SELECT "+columns+" FROM pages WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []Page{}
	for rows.Next() {
		p, e := scanPage(rows)
		if e != nil {
			return nil, e
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
func (s *Store) Page(id int64) (Page, error) {
	return scanPage(s.DB.QueryRow("SELECT "+pageColumns+" FROM pages WHERE id=?", id))
}
func (s *Store) Public(slug string) (Page, error) {
	var p Page
	var termJSON, seoJSON string
	err := s.DB.QueryRow("SELECT id,published_slug,version,published_title,published_document,coalesce(published_cover,''),kind,published_term_ids,published_at,published_seo,published_updated_at,owner_id,published_parent_id,published_sort_order,published_template FROM pages WHERE trashed_at=0 AND published_slug=?", slug).Scan(&p.ID, &p.Slug, &p.Version, &p.Title, &p.Document, &p.Cover, &p.Kind, &termJSON, &p.PublishedAt, &seoJSON, &p.PublishedUpdatedAt, &p.OwnerID, &p.ParentID, &p.Order, &p.Template)
	if err == nil {
		err = json.Unmarshal([]byte(termJSON), &p.TermIDs)
		if err == nil {
			err = json.Unmarshal([]byte(seoJSON), &p.SEO)
		}
	}
	return p, err
}
func (s *Store) Save(ctx context.Context, p Page, publish bool) (Page, error) {
	auditAction := "update"
	if p.ID == 0 {
		auditAction = "create"
	}
	if publish {
		auditAction = "publish"
	}
	if p.Kind == "" {
		p.Kind = "page"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	if p.Template == "" {
		p.Template = "page.html"
	}
	p.Slug, err = pagePath(ctx, tx, p.ID, p.ParentID, p.Slug, p.Kind)
	if err != nil {
		return p, err
	}
	if p.TermIDs == nil {
		p.TermIDs = []int64{}
	}
	termJSON, e := json.Marshal(p.TermIDs)
	if e != nil {
		return p, e
	}
	if len(p.TermIDs) > 0 {
		kind := p.Kind
		if p.ID > 0 {
			if err = tx.QueryRowContext(ctx, "SELECT kind FROM pages WHERE id=?", p.ID).Scan(&kind); err != nil {
				return p, err
			}
		}
		var n int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM terms WHERE deleted_at=0 AND id IN(SELECT value FROM json_each(?))", string(termJSON)).Scan(&n); err != nil {
			return p, err
		}
		if kind != "post" || n != len(p.TermIDs) {
			return p, ErrInvalidTerms
		}
	}
	seoJSON, e := json.Marshal(p.SEO)
	if e != nil {
		return p, e
	}
	if p.ID == 0 {
		if p.Version != 0 {
			return p, ErrConflict
		}
		res, e := tx.ExecContext(ctx, "INSERT INTO pages(slug,version,title,document,cover,kind,created_at,updated_at,term_ids,seo,owner_id,parent_id,sort_order,template) VALUES(?,1,?,?,nullif(?,''),?,unixepoch(),unixepoch(),?,?,?,?,?,?)", p.Slug, p.Title, p.Document, p.Cover, p.Kind, string(termJSON), string(seoJSON), Owner(ctx), p.ParentID, p.Order, p.Template)
		if e != nil {
			return p, e
		}
		p.ID, err = res.LastInsertId()
		if err != nil {
			return p, err
		}
		p.Version = 1
	} else {
		res, e := tx.ExecContext(ctx, "UPDATE pages SET slug=?,title=?,document=?,cover=nullif(?,''),term_ids=?,seo=?,parent_id=?,sort_order=?,template=?,version=version+1,updated_at=unixepoch() WHERE id=? AND version=? AND trashed_at=0", p.Slug, p.Title, p.Document, p.Cover, string(termJSON), string(seoJSON), p.ParentID, p.Order, p.Template, p.ID, p.Version)
		if e != nil {
			return p, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return p, e
		}
		if n != 1 {
			return p, ErrConflict
		}
		p.Version++
	}
	if err = reserveRoute(ctx, tx, p.Slug, p.ID); err != nil {
		return p, err
	}
	if publish {
		if err = publishCurrent(ctx, tx, p.ID, p.Version); err != nil {
			return p, err
		}
	}
	if _, err = tx.ExecContext(ctx, revisionSQL, p.ID); err != nil {
		return p, err
	}
	if err = auditContent(ctx, tx, auditAction, p); err != nil {
		return p, err
	}
	if err = tx.Commit(); err != nil {
		return p, err
	}
	return s.Page(p.ID)
}

const publishSQL = "UPDATE pages SET published_parent_id=parent_id,published_sort_order=sort_order,published_template=template,published_slug=slug,published_title=title,published_document=document,published_cover=cover,published_term_ids=term_ids,published_seo=seo,published_updated_at=unixepoch(),scheduled_at=0,scheduled_version=0,published_at=CASE WHEN published_slug IS NULL THEN unixepoch() ELSE published_at END,updated_at=unixepoch(),version=version+1 WHERE id=? AND version=? AND trashed_at=0"

const revisionSQL = "INSERT INTO revisions(page_id,version,slug,title,document,cover,created_at,term_ids,seo,parent_id,sort_order,template) SELECT id,version,slug,title,document,cover,updated_at,term_ids,seo,parent_id,sort_order,template FROM pages WHERE id=?"

func (s *Store) Publish(ctx context.Context, id, version int64) error {
	return s.ChangeContent(ctx, id, version, "publish")
}

// ChangeContent applies the same version guard to every lifecycle transition.
func (s *Store) ChangeContent(ctx context.Context, id, version int64, action string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = changeContent(ctx, tx, id, version, action); err != nil {
		return err
	}
	return tx.Commit()
}

func changeContent(ctx context.Context, tx *sql.Tx, id, version int64, action string) error {
	var target Page
	target.ID = id
	if err := tx.QueryRowContext(ctx, "SELECT title,kind FROM pages WHERE id=?", id).Scan(&target.Title, &target.Kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		return err
	}

	if action == "publish" {
		if err := publishCurrent(ctx, tx, id, version); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, revisionSQL, id)
		if err != nil {
			return err
		}
		return auditContent(ctx, tx, action, target)
	}
	if action == "trash" || action == "unpublish" || action == "delete" {
		var children int
		query := "SELECT count(*) FROM pages WHERE published_parent_id=? AND published_slug IS NOT NULL AND trashed_at=0"
		args := []any{id}
		if action == "delete" {
			query = "SELECT count(*) FROM pages WHERE parent_id=? OR published_parent_id=?"
			args = append(args, id)
		}
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&children); err != nil {
			return err
		}
		if children > 0 {
			return ErrConflict
		}
	}
	query := publishSQL
	switch action {
	case "publish":
	case "unpublish", "trash":
		query = "UPDATE pages SET published_slug=NULL,published_title=NULL,published_document=NULL,published_cover=NULL,published_term_ids='[]',published_seo='{}',published_updated_at=0,scheduled_at=0,scheduled_version=0,published_at=0,updated_at=unixepoch(),version=version+1"
		if action == "trash" {
			query += ",trashed_at=unixepoch()"
		}
		query += " WHERE id=? AND version=? AND trashed_at=0"
	case "cancel_schedule":
		query = "UPDATE pages SET scheduled_at=0,scheduled_version=0,updated_at=unixepoch(),version=version+1 WHERE id=? AND version=? AND trashed_at=0"
	case "delete":
		if _, err := tx.ExecContext(ctx, "DELETE FROM routes WHERE page_id=? AND EXISTS(SELECT 1 FROM pages WHERE id=? AND version=? AND trashed_at>0)", id, id, version); err != nil {
			return err
		}
		query = "DELETE FROM pages WHERE id=? AND version=? AND trashed_at>0"
	case "restore":
		query = "UPDATE pages SET trashed_at=0,updated_at=unixepoch(),version=version+1 WHERE id=? AND version=? AND trashed_at>0"
	default:
		return errors.New("invalid content action")
	}
	result, err := tx.ExecContext(ctx, query, id, version)
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
	if action != "delete" {
		_, err = tx.ExecContext(ctx, revisionSQL, id)
	}
	if err != nil {
		return err
	}
	return auditContent(ctx, tx, action, target)
}

// ChangeContents keeps a selection atomic and checks each submitted version.
func (s *Store) ChangeContents(ctx context.Context, kind, action string, items []Page) error {
	if (kind != "page" && kind != "post") || (action != "trash" && action != "restore" && action != "delete") || len(items) == 0 || len(items) > ContentPageSize {
		return errors.New("invalid content selection")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if action == "trash" || action == "delete" {
		depths := map[int64]int{}
		for _, item := range items {
			var slug string
			if err = tx.QueryRowContext(ctx, "SELECT slug FROM pages WHERE id=?", item.ID).Scan(&slug); err != nil {
				return err
			}
			depths[item.ID] = len(slug)
		}
		items = append([]Page(nil), items...)
		sort.SliceStable(items, func(i, j int) bool { return depths[items[i].ID] > depths[items[j].ID] })
	}
	for _, item := range items {
		var actual string
		if err = tx.QueryRowContext(ctx, "SELECT kind FROM pages WHERE id=?", item.ID).Scan(&actual); err != nil {
			return err
		}
		if actual != kind {
			return ErrConflict
		}
		if err = changeContent(ctx, tx, item.ID, item.Version, action); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Revision struct {
	Version   int64
	Title     string
	CreatedAt int64
}

func (s *Store) Revisions(id int64) ([]Revision, error) {
	rows, err := s.DB.Query("SELECT version,title,created_at FROM revisions WHERE page_id=? ORDER BY version DESC LIMIT 50", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Revision{}
	for rows.Next() {
		var r Revision
		if err = rows.Scan(&r.Version, &r.Title, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Store) Revision(id, version int64) (Page, error) {
	var p Page
	var terms, seoJSON string
	err := s.DB.QueryRow("SELECT page_id,version,slug,title,document,coalesce(cover,''),term_ids,seo,(SELECT kind FROM pages WHERE id=page_id),parent_id,sort_order,template FROM revisions WHERE page_id=? AND version=?", id, version).Scan(&p.ID, &p.Version, &p.Slug, &p.Title, &p.Document, &p.Cover, &terms, &seoJSON, &p.Kind, &p.ParentID, &p.Order, &p.Template)
	if err == nil {
		err = json.Unmarshal([]byte(terms), &p.TermIDs)
		if err == nil {
			err = json.Unmarshal([]byte(seoJSON), &p.SEO)
		}
	}
	return p, err
}
func (s *Store) Snapshot(dest string) error { _, err := s.DB.Exec("VACUUM INTO ?", dest); return err }

// CheckFile never initializes a database and uses SQLite's read-only URI mode.
func CheckFile(file string) error {
	u := url.URL{Scheme: "file", Path: file}
	u.RawQuery = "mode=ro"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err = s.Check(); err != nil {
		return err
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 1 || version > 14 {
		return errors.New("unsupported schema")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM installation").Scan(&n); err != nil {
		return err
	}
	rows, err := db.Query("SELECT name,size FROM media")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var size int64
		if err = rows.Scan(&name, &size); err != nil {
			return err
		}
		if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
			return errors.New("invalid media reference")
		}
		info, e := os.Lstat(filepath.Join(filepath.Dir(file), "uploads", name))
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Size() != size {
			return errors.New("media file missing or wrong size")
		}
	}
	return rows.Err()
}

// PageSummary counts site content independently of the bounded page list.
type PageSummary struct{ Total, Published, Drafts int }

func (s *Store) PageSummary(kinds ...string) (PageSummary, error) {
	kind := "page"
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	var summary PageSummary
	err := s.DB.QueryRow("SELECT count(*),coalesce(sum(published_slug IS NOT NULL),0),coalesce(sum(published_slug IS NULL),0) FROM pages WHERE kind=? AND trashed_at=0", kind).Scan(&summary.Total, &summary.Published, &summary.Drafts)
	return summary, err
}

// PublishedContent exposes only the published snapshot to public themes.
func (s *Store) PublishedContent(kind string) ([]Page, error) {
	rows, err := s.DB.Query("SELECT published_slug,published_title,coalesce(published_cover,'') FROM pages WHERE kind=? AND trashed_at=0 AND published_slug IS NOT NULL ORDER BY published_sort_order,published_title,id LIMIT 100", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []Page{}
	for rows.Next() {
		var p Page
		if err = rows.Scan(&p.Slug, &p.Title, &p.Cover); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
