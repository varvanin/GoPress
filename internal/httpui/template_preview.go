package httpui

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"testing/fstest"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

type templatePreviewFS struct {
	Base         fs.FS
	Prefix, Name string
	Content      []byte
	Embedded     bool
}

func (p templatePreviewFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	rel := strings.TrimPrefix(name, p.Prefix)
	if rel == p.Name {
		data := p.Content
		if p.Embedded && strings.HasSuffix(rel, ".html") {
			data = templateAssetURLs(data)
		}
		return (fstest.MapFS{name: &fstest.MapFile{Data: data, Mode: 0600}}).Open(name)
	}
	if p.Embedded && strings.HasPrefix(rel, "assets/") {
		return resources.Open(rel)
	}
	if p.Embedded && strings.HasSuffix(rel, ".html") {
		data, err := fs.ReadFile(p.Base, name)
		if err != nil {
			return nil, err
		}
		return (fstest.MapFS{name: &fstest.MapFile{Data: templateAssetURLs(data)}}).Open(name)
	}
	return p.Base.Open(name)
}

type templatePreview struct {
	Owner                       int64
	Expires                     time.Time
	FS                          fs.FS
	Prefix, Theme, Page, Assets string
}

func (a *App) putTemplatePreview(value templatePreview) string {
	a.templatePreviewMu.Lock()
	defer a.templatePreviewMu.Unlock()
	if a.templatePreviews == nil {
		a.templatePreviews = map[string]templatePreview{}
	}
	now := time.Now()
	for token, entry := range a.templatePreviews {
		if !entry.Expires.After(now) {
			delete(a.templatePreviews, token)
		}
	}
	for len(a.templatePreviews) >= 16 {
		oldest := ""
		var expires time.Time
		for token, entry := range a.templatePreviews {
			if oldest == "" || entry.Expires.Before(expires) {
				oldest = token
				expires = entry.Expires
			}
		}
		delete(a.templatePreviews, oldest)
	}
	token := service.Token()
	value.Expires = now.Add(10 * time.Minute)
	value.Assets = "/admin/themes/editor/previews/" + token + "/assets"
	a.templatePreviews[token] = value
	return token
}
func (a *App) getTemplatePreview(token string, owner int64) (templatePreview, bool) {
	a.templatePreviewMu.Lock()
	defer a.templatePreviewMu.Unlock()
	entry, ok := a.templatePreviews[token]
	if !ok || !entry.Expires.After(time.Now()) {
		delete(a.templatePreviews, token)
		return templatePreview{}, false
	}
	return entry, entry.Owner == owner
}
func (a *App) previewTemplateFile(c *echo.Context) error {
	id, name, content := c.FormValue("theme"), c.FormValue("file"), c.FormValue("content")
	if !editableTemplateFile(name) || len(content) > 512<<10 || !utf8.ValidString(content) {
		return service.ErrInvalid
	}
	a.themeMu.RLock()
	entry, err := func() (templatePreview, error) {
		files, _, size, err := a.templateFiles(id, name, true)
		if err != nil {
			return templatePreview{}, err
		}
		old, ok := files[name]
		if !ok {
			return templatePreview{}, service.ErrInvalid
		}
		if templateFileHash(old.Data) != c.FormValue("revision") {
			return templatePreview{}, store.ErrConflict
		}
		if size-len(old.Data)+len(content) > 32<<20 {
			return templatePreview{}, service.ErrInvalid
		}
		files[name] = &fstest.MapFile{Data: []byte(content), Mode: 0600}
		if err = validateThemeFiles(files, "", store.Appearance{Theme: id}); err != nil {
			return templatePreview{}, err
		}
		source, prefix, _, err := a.themeSource(id)
		if err != nil {
			return templatePreview{}, err
		}
		page := "home.html"
		if strings.HasSuffix(name, ".html") && !strings.Contains(name, "/") && name != "layout.html" && name != "header.html" && name != "footer.html" {
			page = name
		}
		user, _ := store.Actor(c.Request().Context())
		return templatePreview{Owner: user.ID, FS: templatePreviewFS{Base: source, Prefix: prefix, Name: name, Content: []byte(content), Embedded: prefix != ""}, Prefix: prefix, Theme: id, Page: page}, nil
	}()
	a.themeMu.RUnlock()
	if err != nil {
		return err
	}
	token := a.putTemplatePreview(entry)
	return c.Redirect(303, "/admin/themes/editor/previews/"+token)
}
func (a *App) showTemplatePreview(c *echo.Context) error {
	user, _ := store.Actor(c.Request().Context())
	entry, ok := a.getTemplatePreview(c.Param("preview"), user.ID)
	if !ok {
		return echo.NewHTTPError(404, "Preview not found")
	}
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	appearance.Theme = entry.Theme
	p := store.Page{Document: emptyDocument, Template: entry.Page}
	if entry.Page == "single.html" {
		posts, _, e := a.Service.Store.PublicPosts(0, 1, 1)
		if e != nil {
			return e
		}
		if len(posts) > 0 {
			p, err = a.Service.Store.Public(posts[0].Slug)
		}
	} else if entry.Page != "home.html" && entry.Page != "archive.html" && entry.Page != "search.html" {
		pages, e := a.Service.Store.HomePages()
		if e != nil {
			return e
		}
		if len(pages) > 0 {
			p, err = a.Service.Store.PublicPage(pages[0].ID)
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		p = store.Page{Document: emptyDocument}
	} else if err != nil {
		return err
	}
	if entry.Page == "search.html" {
		c.Set("public-search", &PublicSearchData{Page: 1, Language: c.QueryParam("lang")})
	}
	c.Set("template-preview", &entry)
	return a.render(c, p, entry.Page == "404.html", appearance)
}
func (a *App) previewTemplateAsset(c *echo.Context) error {
	user, _ := store.Actor(c.Request().Context())
	entry, ok := a.getTemplatePreview(c.Param("preview"), user.ID)
	if !ok {
		return echo.NewHTTPError(404, "Preview not found")
	}
	name := c.Param("*")
	if !fs.ValidPath(name) || !strings.HasPrefix(name, "assets/") || strings.ContainsAny(name, "\\:") {
		return echo.NewHTTPError(404, "Preview not found")
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".css", ".js", ".png", ".jpg", ".jpeg", ".webp", ".svg", ".ico", ".woff", ".woff2":
	default:
		return echo.NewHTTPError(404, "Preview not found")
	}
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	file, err := entry.FS.Open(entry.Prefix + name)
	if err != nil {
		return echo.NewHTTPError(404, "Preview not found")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return echo.NewHTTPError(404, "Preview not found")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Type", mime.TypeByExtension(ext))
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Response(), c.Request(), path.Base(name), info.ModTime(), seeker)
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(file, 8<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return service.ErrInvalid
	}
	return c.Blob(200, mime.TypeByExtension(ext), data)
}
