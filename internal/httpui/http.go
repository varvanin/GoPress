package httpui

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

//go:embed assets/* theme/* theme-minimal/* theme-studio/* starter/*
var resources embed.FS

// ThemeData deliberately contains no database, session, email, or password fields.
type ThemeData struct {
	Template                 string
	Archive, NotFound        bool
	Pagination               ThemePagination
	Search                   *PublicSearchData
	Kind, Slug               string
	ThemeAssets              string
	Year, ReadingMinutes     int
	Description              string
	Featured                 *PostView
	Spotlight, Feed, Related []PostView
	Categories               []store.Term
	ActiveTerm               *store.Term
	Appearance               store.Appearance
	SEO                      PageMeta
	Preview                  bool
	HeaderMenu, FooterMenu   []MenuLink
	HeaderMenuConfigured     bool
	PublishedAt              int64
	Terms                    []store.Term
	PostTerms                []store.Term
	BlogPage, BlogTotal      int
	BlogPageSize             int
	BlogTerm                 int64
	Home                     bool
	HomeIsPage               bool
	Pages                    []store.Page
	Posts                    []store.Page
	Site                     ThemeSite
	Title                    string
	Cover                    string
	Body                     template.HTML
}

type ThemeSite struct{ Title, BaseURL, Timezone, Description string }
type App struct {
	themeMu sync.RWMutex
	Service *service.Service
	Echo    *echo.Echo
}

func New(v *service.Service) *App {
	a := &App{Service: v, Echo: echo.New()}
	e := a.Echo
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(a.operationGuard)
	e.Use(middleware.Recover())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			h := c.Response().Header()
			requestID := service.Token()[:16]
			h.Set("X-Request-ID", requestID)
			switch c.Request().URL.Path {
			case "/install", "/login", "/logout":
				h.Set("Cache-Control", "no-store")
				h.Set("X-Robots-Tag", "noindex, nofollow")
			}
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			if strings.HasPrefix(c.Request().URL.Path, "/admin") {
				nonce := service.Token()
				c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), styleNonceKey{}, nonce)))
				h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'nonce-"+nonce+"'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			}

			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, requestBodyLimit(c.Request().URL.Path))
			err := next(c)
			slog.Info("request", "id", requestID, "method", c.Request().Method, "route", c.Path(), "duration_ms", time.Since(start).Milliseconds())
			return err
		}
	})
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		code, msg := 500, "Internal server error"
		var he interface{ StatusCode() int }
		switch {
		case errors.As(err, &he):
			code = he.StatusCode()
			msg = http.StatusText(code)
		case errors.Is(err, service.ErrInvalid):
			code, msg = 400, "Invalid input"
		case errors.Is(err, service.ErrForbidden):
			code, msg = 403, "Forbidden"
		case errors.Is(err, store.ErrLastAdmin):
			code, msg = 409, "At least one active administrator is required"
		case errors.Is(err, service.ErrUnauthorized):
			code, msg = 401, "Invalid credentials"
		case errors.Is(err, service.ErrRateLimit):
			code, msg = 429, "Too many attempts; retry in 15 minutes"
			c.Response().Header().Set("Retry-After", "900")
		case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrInstalled):
			code, msg = 409, "Version, URL or installation conflict"
		case errors.Is(err, sql.ErrNoRows):
			code, msg = 404, "Not found"
		default:
			if strings.Contains(err.Error(), "constraint failed") {
				code, msg = 409, "Conflicting URL or reference"
			} else {
				slog.Error("request failed", "route", c.Path())
			}
		}
		_ = c.String(code, msg)
	}
	e.GET("/healthz", func(c *echo.Context) error { return c.String(200, "ok") })
	e.GET("/readyz", func(c *echo.Context) error {
		if err := v.Store.DB.PingContext(c.Request().Context()); err != nil {
			return echo.NewHTTPError(503, "Unavailable")
		}
		return c.String(200, "ok")
	})
	assets, _ := fs.Sub(resources, "assets")
	e.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))))
	e.GET("/install", a.installForm)
	e.GET("/theme-assets/:theme/*", a.themeAsset)
	e.POST("/install", a.install, a.csrf)
	e.GET("/login", a.loginForm)
	e.POST("/login", a.login, a.csrf)
	e.POST("/logout", func(c *echo.Context) error {
		cookie, _ := c.Cookie("session")
		if err := v.Store.Logout(service.Hash(cookie.Value)); err != nil {
			return err
		}
		a.cookie(c, "session", "", -1)
		return c.Redirect(303, "/login")
	}, a.auth, a.csrf)
	admin := e.Group("/admin", a.auth, a.permissions)
	admin.GET("", a.dashboard)
	admin.GET("/translations.js", func(c *echo.Context) error {
		raw, err := json.Marshal(i18n.Catalog(i18n.Language(c.Request().Context())))
		if err != nil {
			return err
		}
		return c.Blob(200, "text/javascript; charset=utf-8", append([]byte("globalThis.GoPressTranslations="), raw...))
	})
	admin.GET("/csrf", func(c *echo.Context) error { return c.JSON(200, map[string]string{"csrf": a.csrfToken(c)}) })
	a.adminRoutes(admin)
	a.appearanceRoutes(admin)
	a.seoRoutes()
	admin.GET("/pages", a.contentList)
	admin.GET("/pages/:id", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		p, err := v.Store.Page(id)
		if err != nil {
			return err
		}
		return c.JSON(200, p)
	})
	save := func(c *echo.Context) error {
		var p store.Page
		if err := decode(c, &p); err != nil {
			return err
		}
		pathID := int64(0)
		if c.Param("id") != "" {
			var err error
			pathID, err = pageID(c)
			if err != nil {
				return err
			}
		}
		if p.ID != pathID {
			return service.ErrInvalid
		}
		if err := a.checkPageTemplate(p); err != nil {
			return err
		}
		p, err := v.Save(c.Request().Context(), p, false)
		if err != nil {
			return err
		}
		return c.JSON(200, p)
	}
	admin.POST("/pages", save, a.csrf)
	admin.POST("/pages/:id", save, a.csrf)
	admin.POST("/pages/:id/publish", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		var p struct {
			Version int64 `json:"version"`
		}
		if err = decode(c, &p); err != nil {
			return err
		}
		if err = v.Store.Publish(c.Request().Context(), id, p.Version); err != nil {
			return err
		}
		return c.NoContent(204)
	}, a.csrf)
	admin.GET("/pages/:id/preview", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		p, err := v.Store.Page(id)
		if err != nil {
			return err
		}
		return a.render(c, p, false)
	})
	admin.GET("/media", func(c *echo.Context) error {
		c.Response().Header().Add("Vary", "Accept")
		if strings.Contains(c.Request().Header.Get("Accept"), "text/html") || c.Request().Header.Get("HX-Request") == "true" {
			return a.mediaLibrary(c, 200, "", nil)
		}
		names, err := v.Store.MediaNames(mediaOwner(c.Request().Context()))
		if err != nil {
			return err
		}
		return c.JSON(200, names)
	})
	admin.POST("/media", func(c *echo.Context) error {
		name, err := v.Upload(c.Request().Body, c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(201, map[string]string{"name": name})
	}, a.csrf)
	e.GET("/uploads/:name", func(c *echo.Context) error {
		name := c.Param("name")
		if filepath.Base(name) != name || strings.ContainsAny(name, "\\/") {
			return echo.NewHTTPError(404, "Not found")
		}
		mime, err := v.Store.MediaType(name)
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Type", mime)
		return c.FileFS(name, os.DirFS(filepath.Join(v.Store.Dir, "uploads")))
	})
	e.GET("/", func(c *echo.Context) error {
		site, err := v.Store.Site()
		if errors.Is(err, sql.ErrNoRows) {
			return c.Redirect(303, "/install")
		}
		if err != nil {
			return err
		}
		return a.render(c, store.Page{Title: site.Title, Document: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Welcome to your independent website."}]}]}`}, false)
	})
	e.GET("/search", a.search)
	e.GET("/*", func(c *echo.Context) error {
		slug := strings.Trim(c.Request().URL.Path, "/")
		normalized := strings.ToLower(slug)
		if normalized != "" && (slug != normalized || c.Request().URL.Path != "/"+normalized) {
			return c.Redirect(301, contentRedirectURL(normalized, c.Request().URL.RawQuery))
		}
		p, err := v.Store.Public(normalized)
		if errors.Is(err, sql.ErrNoRows) {
			target, e := v.Store.Redirect(normalized)
			if e == nil && target != normalized {
				return c.Redirect(301, contentRedirectURL(target, c.Request().URL.RawQuery))
			}
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			return a.render(c, store.Page{}, true)
		}
		if err != nil {
			return err
		}
		return a.render(c, p, false)
	})
	return a
}
func requestBodyLimit(path string) int64 {
	switch {
	case path == "/admin/tools/backups/import":
		return 1 << 30
	case path == "/admin/pages/languages":
		return 16 << 10
	case strings.HasPrefix(path, "/forms/"):
		return 128 << 10
	case path == "/admin/pages" || strings.HasPrefix(path, "/admin/pages/"):
		return service.MaxContentRequestSize
	default:
		return service.MaxUpload + 1<<20
	}
}
func contentRedirectURL(path string, query string) string {
	return (&url.URL{Path: "/" + path, RawQuery: query}).String()
}
func decode(c *echo.Context, v any) error {
	if !strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/json") {
		return service.ErrInvalid
	}
	d := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, requestBodyLimit(c.Request().URL.Path)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return service.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return service.ErrInvalid
	}
	return nil
}
func pageID(c *echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, service.ErrInvalid
	}
	return id, nil
}
func (a *App) cookie(c *echo.Context, name, value string, maxAge int) {
	secure := c.Request().TLS != nil
	if s, err := a.Service.Store.Site(); err == nil {
		secure = strings.HasPrefix(s.BaseURL, "https://")
	}
	c.SetCookie(&http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
}
func (a *App) csrfToken(c *echo.Context) string {
	if session, err := c.Cookie("session"); err == nil && len(session.Value) == 64 {
		token := service.Hash("csrf:" + session.Value)
		a.cookie(c, "csrf", token, 43200)
		return token
	}
	token := ""
	if cookie, err := c.Cookie("csrf"); err == nil && len(cookie.Value) == 64 {
		token = cookie.Value
	}
	if token == "" {
		token = service.Token()
		a.cookie(c, "csrf", token, 43200)
	}
	return token
}
func (a *App) csrf(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cookie, err := c.Cookie("csrf")
		if err != nil || len(cookie.Value) != 64 {
			return echo.NewHTTPError(403, "CSRF")
		}
		token := c.Request().Header.Get("X-CSRF-Token")
		if token == "" && strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			token = c.FormValue("csrf")
		}
		if token == "" && strings.HasPrefix(c.Request().Header.Get("Content-Type"), "multipart/form-data") {
			if err := c.Request().ParseMultipartForm(1 << 20); err != nil {
				return service.ErrInvalid
			}
			defer c.Request().MultipartForm.RemoveAll()
			token = c.Request().FormValue("csrf")
		}
		if !service.Equal(token, cookie.Value) {
			return echo.NewHTTPError(403, "CSRF")
		}
		if session, e := c.Cookie("session"); e == nil && len(session.Value) == 64 && !service.Equal(token, service.Hash("csrf:"+session.Value)) {
			return echo.NewHTTPError(403, "CSRF")
		}
		origin := c.Request().Header.Get("Origin")
		if origin != "" {
			expected := "http://" + c.Request().Host
			if c.Request().TLS != nil {
				expected = "https://" + c.Request().Host
			}
			if site, e := a.Service.Store.Site(); e == nil {
				expected = site.BaseURL
			}
			if origin != expected {
				return echo.NewHTTPError(403, "Origin")
			}
		}
		return next(c)
	}
}
func (a *App) auth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
		cookie, err := c.Cookie("session")
		if err != nil {
			if strings.Contains(c.Request().Header.Get("Accept"), "text/html") || c.Request().Header.Get("HX-Request") == "true" {
				if c.Request().Header.Get("HX-Request") == "true" {
					c.Response().Header().Set("HX-Redirect", "/login")
					return c.NoContent(204)
				}
				return c.Redirect(303, "/login")
			}
			return echo.NewHTTPError(401, "Unauthorized")
		}
		user, err := a.Service.Store.SessionUser(service.Hash(cookie.Value))
		ok := err == nil
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return err
		}
		if !ok {
			if strings.Contains(c.Request().Header.Get("Accept"), "text/html") || c.Request().Header.Get("HX-Request") == "true" {
				if c.Request().Header.Get("HX-Request") == "true" {
					c.Response().Header().Set("HX-Redirect", "/login")
					return c.NoContent(204)
				}
				return c.Redirect(303, "/login")
			}
			return echo.NewHTTPError(401, "Unauthorized")
		}
		settings, err := a.Service.Store.Settings()
		if err != nil {
			return err
		}
		ctx := store.WithActor(i18n.WithLanguage(c.Request().Context(), settings.Language), user)
		if user.Role == "admin" {
			modules, err := a.moduleCatalog()
			if err != nil {
				return err
			}
			enabled := make([]ModuleInfo, 0, len(modules))
			for _, module := range modules {
				if module.Enabled {
					enabled = append(enabled, module)
				}
			}
			ctx = context.WithValue(ctx, moduleNavigationKey{}, moduleNavigation{Modules: enabled, Path: c.Request().URL.Path})
		}
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}
func (a *App) installForm(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Robots-Tag", "noindex")
	if _, err := a.Service.Store.Site(); err == nil {
		return echo.NewHTTPError(409, "Installed")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return System("Установить GoPress", a.csrfToken(c), true).Render(c.Request().Context(), c.Response())
}
func (a *App) loginForm(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Robots-Tag", "noindex")
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	settings, err := a.Service.Store.Settings()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return System("Войти", a.csrfToken(c), false).Render(i18n.WithLanguage(c.Request().Context(), settings.Language), c.Response())
}
func (a *App) install(c *echo.Context) error {
	err := a.Service.Install(c.Request().Context(), c.FormValue("token"), store.Site{Title: c.FormValue("title"), BaseURL: c.FormValue("base_url"), Timezone: c.FormValue("timezone")}, c.FormValue("email"), c.FormValue("password"))
	if err != nil {
		return err
	}
	return c.Redirect(303, "/login")
}
func (a *App) login(c *echo.Context) error {
	token, err := a.Service.Login(c.RealIP(), c.FormValue("email"), c.FormValue("password"))
	if err != nil {
		return err
	}
	if old, e := c.Cookie("session"); e == nil {
		if err = a.Service.Store.Logout(service.Hash(old.Value)); err != nil {
			return err
		}
	}
	a.cookie(c, "session", token, 43200)
	a.cookie(c, "csrf", service.Hash("csrf:"+token), 43200)
	return c.Redirect(303, "/admin")
}
func (a *App) render(c *echo.Context, p store.Page, notFound bool, preview ...store.Appearance) error {
	settings, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	site := settings.Site
	staticHome := false
	if c.Request().URL.Path == "/" && len(preview) == 0 && settings.HomeMode == "page" && !notFound {
		p, err = a.Service.Store.PublicPage(settings.HomePageID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		staticHome = err == nil
		if !staticHome {
			p = store.Page{Title: site.Title, Document: emptyDocument}
		}
	}
	originalLanguage := sourceContentLanguage(p)
	language := c.QueryParam("lang")
	if language != "" && language != "original" && !notFound {
		translation, exists := p.Translations[language]
		if !exists {
			return echo.NewHTTPError(http.StatusNotFound, "Translation not found")
		}
		p.Title, p.Document, p.SEO = translation.Title, translation.Document, translation.SEO
	}
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	if len(preview) > 0 {
		appearance = preview[0]
	}
	source, prefix, resolved, err := a.themeSource(appearance.Theme)
	if err != nil {
		return err
	}
	appearance.Theme = resolved
	if appearance.HomeDescription == "" {
		appearance.HomeDescription = settings.Description
	}
	private := strings.HasPrefix(c.Request().URL.Path, "/admin/")
	body := ""
	status := 200
	page := "page.html"
	if p.Kind == "post" {
		page = "single.html"
	}
	if p.Kind == "page" && service.ValidPageTemplate(p.Template) {
		if _, e := fs.Stat(source, prefix+p.Template); e == nil {
			page = p.Template
		} else if !errors.Is(e, fs.ErrNotExist) {
			return e
		}
	}
	if notFound {
		status = 404
		p.Title = "Страница не найдена"
		page = "404.html"
	} else {
		body, err = service.Render(p.Document)
		if err != nil {
			return err
		}
	}
	body, err = a.renderModules(c, body)
	if err != nil {
		return err
	}
	body, err = a.renderCollections(c, body)
	if err != nil {
		return err
	}
	body, err = a.renderFAQ(c, body)
	if err != nil {
		return err
	}
	body, err = a.renderForms(c, body)
	if err != nil {
		return err
	}
	search, _ := c.Get("public-search").(*PublicSearchData)
	if search != nil {
		body, err = searchBody(c, search)
		if err != nil {
			return err
		}
	}
	home := (c.Request().URL.Path == "/" || len(preview) > 0) && !notFound
	if home && p.Title == "" {
		p.Title = site.Title
	}
	pages, err := a.Service.Store.PublishedContent("page")
	if err != nil {
		return err
	}
	headerMenu, err := a.Service.Store.Menu("header")
	if err != nil {
		return err
	}
	footerMenu, err := a.Service.Store.Menu("footer")
	if err != nil {
		return err
	}
	blogPage := 1
	var blogTerm int64
	if home {
		if raw := c.QueryParam("page"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > 1000000 {
				return service.ErrInvalid
			}
			blogPage = n
		}
		if raw := c.QueryParam("term"); raw != "" {
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || n < 1 {
				return service.ErrInvalid
			}
			blogTerm = n
		}
	}
	posts, total, err := a.Service.Store.PublicPosts(blogTerm, blogPage, settings.PostsPerPage)
	if err != nil {
		return err
	}
	terms, err := a.Service.Store.Terms(home || !strings.HasPrefix(c.Request().URL.Path, "/admin/"))
	if err != nil {
		return err
	}
	postTerms := []store.Term{}
	for _, term := range terms {
		if selectedTerm(p.TermIDs, term.ID) {
			postTerms = append(postTerms, term)
		}
	}
	zone, e := time.LoadLocation(site.Timezone)
	if e != nil {
		zone = time.UTC
	}
	meta := pageMetadata(site, appearance, p, home && (language == "" || language == "original"), private, notFound, blogPage, blogTerm)
	if settings.HomeMode == "page" && p.ID == settings.HomePageID && !private && !notFound {
		meta.Canonical = site.BaseURL + "/"
	}
	meta.ContentLanguage = originalLanguage
	if language != "" && language != "original" && !notFound {
		meta.ContentLanguage = language
		if meta.Canonical != "" && p.SEO.Canonical == "" {
			canonical, e := url.Parse(meta.Canonical)
			if e != nil {
				return e
			}
			query := canonical.Query()
			query.Set("lang", language)
			canonical.RawQuery = query.Encode()
			meta.Canonical = canonical.String()
		}
	}
	applySEOOptions(&meta, site, appearance, p, home && (language == "" || language == "original"), private, notFound, blogPage, blogTerm, terms, search, pages)
	if language != "" && language != "original" && !notFound {
		meta.Locale = strings.ReplaceAll(language, "-", "_")
	}
	if !home && meta.Description == "" {
		meta.Description = settings.Description
	}
	if meta.NoIndex {
		c.Response().Header().Set("X-Robots-Tag", meta.Robots)
	}
	archive := home && !staticHome && (blogTerm > 0 || blogPage > 1)
	if home && !staticHome {
		page = "home.html"
		if archive {
			page = "archive.html"
		}
	}
	if search != nil {
		page = "search.html"
	}
	page, err = resolveThemePage(source, prefix, page)
	if err != nil {
		return err
	}
	pagination := themePagination("/", c.QueryParams(), blogPage, total, settings.PostsPerPage)
	if search != nil {
		pagination = themePagination("/search", c.QueryParams(), search.Page, search.Total, 20)
	}
	assets := "/assets"
	if strings.HasPrefix(appearance.Theme, "theme:") {
		assets = "/theme-assets/" + strings.TrimPrefix(appearance.Theme, "theme:")
	}
	t, err := themeTemplates(source, prefix, page, zone, settings.PostsPerPage)
	if err != nil {
		return err
	}
	views := postViews(posts, terms)
	feed := views
	var featured *PostView
	var spotlight, related []PostView
	if home && appearance.Theme != "minimal" && blogTerm == 0 && blogPage == 1 && len(views) > 0 {
		featured = &views[0]
		n := min(3, len(views))
		spotlight = views[1:n]
		feed = views[n:]
	}
	if p.Kind == "post" {
		for _, view := range views {
			if view.Slug != p.Slug {
				related = append(related, view)
			}
			if len(related) == 3 {
				break
			}
		}
	}
	categories := []store.Term{}
	var activeTerm *store.Term
	for _, term := range terms {
		if term.Kind == "category" {
			categories = append(categories, term)
		}
		if term.ID == blogTerm {
			activeTerm = &term
		}
	}
	minutes := 0
	if p.Kind == "post" {
		minutes = service.ReadingMinutes(p.Document)
	}
	var out themeOutput
	// Only the server's strict JSON renderer may construct trusted HTML.
	if err = executeTheme(t, &out, ThemeData{Template: page, Archive: archive, NotFound: notFound, Pagination: pagination, Search: search, ThemeAssets: assets, Kind: p.Kind, Slug: p.Slug, Year: time.Now().Year(), ReadingMinutes: minutes, Description: p.SEO.Description, Featured: featured, Spotlight: spotlight, Feed: feed, Related: related, Categories: categories, ActiveTerm: activeTerm, Appearance: appearance, SEO: meta, Preview: private, HeaderMenu: menuLinks(headerMenu.Items), FooterMenu: menuLinks(footerMenu.Items), HeaderMenuConfigured: headerMenu.Version > 0, PublishedAt: p.PublishedAt, Terms: terms, PostTerms: postTerms, BlogPage: blogPage, BlogTotal: total, BlogPageSize: settings.PostsPerPage, BlogTerm: blogTerm, Home: home, HomeIsPage: settings.HomeMode == "page", Pages: pages, Posts: posts, Site: ThemeSite{Title: site.Title, BaseURL: site.BaseURL, Timezone: site.Timezone, Description: settings.Description}, Title: p.Title, Cover: p.Cover, Body: template.HTML(body)}); err != nil {
		return err
	}
	return c.HTML(status, out.String())
}
