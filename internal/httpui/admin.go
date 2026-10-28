package httpui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

const emptyDocument = `{"type":"doc","content":[{"type":"paragraph"}]}`

func htmlComponent(c *echo.Context, status int, component templ.Component) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := c.Request().Context()
	if cookie, err := c.Cookie("sidebar_state"); err == nil && cookie.Value == "false" {
		ctx = context.WithValue(ctx, sidebarCollapsedKey{}, true)
	}
	c.Response().Header().Add("Vary", "HX-Request, HX-History-Restore-Request")
	if c.Request().Header.Get("HX-Request") == "true" && c.Request().Header.Get("HX-History-Restore-Request") != "true" {
		ctx = context.WithValue(ctx, partialKey{}, true)
	}
	var out bytes.Buffer
	if err := component.Render(ctx, &out); err != nil {
		return err
	}
	return c.Blob(status, "text/html; charset=utf-8", out.Bytes())
}

type ContentList struct {
	Kind, Query string
	Term        int64
	Terms       []store.Term
	Page, Total int
	Trash       bool
}

func contentPath(kind string) string { return contentLabel(kind, "/admin/pages", "/admin/posts") }
func contentListURL(view ContentList, page int) string {
	q := url.Values{}
	if view.Term > 0 {
		q.Set("term", strconv.FormatInt(view.Term, 10))
	}
	if view.Query != "" {
		q.Set("q", view.Query)
	}
	if view.Trash {
		q.Set("trash", "1")
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	path := contentPath(view.Kind)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path
}
func (a *App) contentList(c *echo.Context) error {
	view := ContentList{Kind: "page", Query: strings.TrimSpace(c.QueryParam("q")), Page: 1, Trash: c.QueryParam("trash") == "1"}
	if strings.HasPrefix(c.Path(), "/admin/posts") {
		view.Kind = "post"
	}
	if raw := c.QueryParam("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000000 {
			return service.ErrInvalid
		}
		view.Page = n
	}
	if len(view.Query) > 200 || c.QueryParam("trash") != "" && c.QueryParam("trash") != "1" {
		return service.ErrInvalid
	}
	if value := c.QueryParam("term"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return service.ErrInvalid
		}
		view.Term = n
	}
	pages, total, err := a.Service.Store.ListContent(view.Kind, view.Query, view.Trash, store.ContentPageSize, (view.Page-1)*store.ContentPageSize, view.Term, mediaOwner(c.Request().Context()))
	if err != nil {
		return err
	}
	view.Total = total
	if view.Kind == "post" {
		view.Terms, err = a.Service.Store.Terms(false)
		if err != nil {
			return err
		}
	}
	c.Response().Header().Set("Vary", "Accept, HX-Request, HX-History-Restore-Request")
	c.Response().Header().Set("X-Total-Count", strconv.Itoa(total))
	if !strings.Contains(c.Request().Header.Get("Accept"), "text/html") && c.Request().Header.Get("HX-Request") != "true" {
		return c.JSON(200, pages)
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, 200, Pages(site.Title, a.csrfToken(c), pages, view))
}

type partialKey struct{}

func partialRequest(ctx context.Context) bool {
	value, _ := ctx.Value(partialKey{}).(bool)
	return value
}
func adminSection(title string) string {
	title = i18n.Source(title)
	if title == "Пользователи" || title == "Пользователь" {
		return "/admin/users"
	}
	if title == "Мой профиль" {
		return "/admin/account"
	}
	if title == "Шаблоны" {
		return "/admin/themes"
	}
	if title == "Инструменты" {
		return "/admin/tools"
	}
	if title == "Настройки" {
		return "/admin/settings"
	}
	if title == "Формы" {
		return "/admin/forms"
	}
	if title == "SEO" {
		return "/admin/seo"
	}
	if title == "Медиатека" {
		return "/admin/media"
	}
	if title == "Меню" {
		return "/admin/menus"
	}
	if strings.Contains(title, "Категории") {
		return "/admin/posts/terms"
	}
	if strings.Contains(strings.ToLower(title), "запис") {
		return "/admin/posts"
	}
	if strings.Contains(strings.ToLower(title), "страниц") {
		return "/admin/pages"
	}
	return "/admin"
}
func contentDate(seconds int64) string {
	if seconds == 0 {
		return "—"
	}
	return time.Unix(seconds, 0).UTC().Format("02.01.2006 15:04 UTC")
}

func (a *App) dashboard(c *echo.Context) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	data, err := a.Service.Store.Dashboard()
	if err != nil {
		return err
	}
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	_, _, theme, themeErr := a.themeSource(appearance.Theme)
	if themeErr != nil {
		theme = "unavailable"
	}
	label := i18n.T(c.Request().Context(), themeLabel(theme))
	if strings.HasPrefix(theme, "theme:") {
		themes, e := a.themes()
		if e != nil {
			return e
		}
		for _, item := range themes {
			if item.ID == theme {
				label = item.Name
				break
			}
		}
	}
	return htmlComponent(c, 200, Dashboard(site, a.csrfToken(c), data, appearance, label))
}

func (a *App) editPage(c *echo.Context, p store.Page, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	if status >= 400 && p.ID > 0 {
		current, e := a.Service.Store.Page(p.ID)
		if e != nil {
			return e
		}
		p.Published = current.Published
		p.PublishedSlug = current.PublishedSlug
		p.Kind = current.Kind
		p.CreatedAt = current.CreatedAt
		p.UpdatedAt = current.UpdatedAt
		p.PublishedAt = current.PublishedAt
		p.TrashedAt = current.TrashedAt
	}
	files, total, err := a.mediaSelection("", 1, p.Cover, mediaOwner(c.Request().Context()))
	if err != nil {
		return err
	}

	revisions, err := a.Service.Store.Revisions(p.ID)
	if err != nil {
		return err
	}
	terms := []store.Term{}
	if p.Kind == "post" {
		terms, err = a.Service.Store.Terms(false)
		if err != nil {
			return err
		}
	}
	if p.Template == "" {
		p.Template = "page.html"
	}
	parents, templates, err := a.pageOptions(c.Request().Context(), p)
	if err != nil {
		return err
	}
	scheduleFailed := false
	if p.ID > 0 && p.ScheduledAt == 0 {
		scheduleFailed, err = a.Service.Store.ScheduleFailed(p.ID, p.Version)
		if err != nil {
			return err
		}
	}
	return htmlComponent(c, status, EditPage(site.Title, a.csrfToken(c), p, files, total, revisions, terms, parents, templates, site.Timezone, message, status >= 400, scheduleFailed))
}
func (a *App) adminRoutes(g *echo.Group) {
	a.blogRoutes(g)
	a.formRoutes(g)
	a.moduleRoutes(g)
	a.settingsRoutes(g)
	a.usersRoutes(g)
	a.toolsRoutes(g)
	a.siteRoutes(g)
	g.POST("/content/bulk", func(c *echo.Context) error {
		if err := c.Request().ParseForm(); err != nil {
			return service.ErrInvalid
		}
		kind, action := c.FormValue("kind"), c.FormValue("action")
		if (kind != "page" && kind != "post") || (action != "trash" && action != "restore" && action != "delete") {
			return service.ErrInvalid
		}
		values := c.Request().PostForm["selection"]
		if len(values) == 0 || len(values) > store.ContentPageSize {
			return service.ErrInvalid
		}
		items := make([]store.Page, 0, len(values))
		seen := map[int64]bool{}
		for _, value := range values {
			parts := strings.Split(value, ":")
			if len(parts) != 2 {
				return service.ErrInvalid
			}
			id, e1 := strconv.ParseInt(parts[0], 10, 64)
			version, e2 := strconv.ParseInt(parts[1], 10, 64)
			if e1 != nil || e2 != nil || id < 1 || version < 1 || seen[id] {
				return service.ErrInvalid
			}
			seen[id] = true
			items = append(items, store.Page{ID: id, Version: version})
		}
		if err := a.Service.Store.ChangeContents(c.Request().Context(), kind, action, items); err != nil {
			return err
		}
		return c.Redirect(303, contentListURL(ContentList{Kind: kind, Trash: action != "trash"}, 1))
	}, a.csrf)
	g.POST("/pages/:id/state", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil || version < 1 {
			return service.ErrInvalid
		}
		action := c.FormValue("action")
		if action != "trash" && action != "restore" && action != "unpublish" && action != "cancel_schedule" && action != "delete" {
			return service.ErrInvalid
		}
		p, err := a.Service.Store.Page(id)
		if err != nil {
			return err
		}
		if err = a.Service.Store.ChangeContent(c.Request().Context(), id, version, action); err != nil {
			return err
		}
		return c.Redirect(303, contentListURL(ContentList{Kind: p.Kind, Trash: action == "delete"}, 1))
	}, a.csrf)
	g.GET("/pages/:id/revisions/:revision", func(c *echo.Context) error {
		id, version, err := revisionID(c)
		if err != nil {
			return err
		}
		p, err := a.Service.Store.Revision(id, version)
		if err != nil {
			return err
		}
		return a.render(c, p, false)
	})
	g.POST("/pages/:id/revisions/:revision/restore", func(c *echo.Context) error {
		id, version, err := revisionID(c)
		if err != nil {
			return err
		}
		expected, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil || expected < 1 {
			return service.ErrInvalid
		}
		p, err := a.Service.Store.Revision(id, version)
		if err != nil {
			return err
		}
		p.Version = expected
		p.TermIDs, err = a.Service.Store.ActiveTermIDs(p.TermIDs)
		if err != nil {
			return err
		}
		if _, err = a.Service.Save(c.Request().Context(), p, false); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/pages/"+strconv.FormatInt(id, 10)+"/edit?done=restored")
	}, a.csrf)

	g.GET("/posts", a.contentList)
	g.GET("/posts/new", func(c *echo.Context) error {
		return a.editPage(c, store.Page{Kind: "post", Document: emptyDocument}, 200, "")
	})

	g.GET("/pages/new", func(c *echo.Context) error { return a.editPage(c, store.Page{Document: emptyDocument}, 200, "") })
	g.GET("/pages/:id/edit", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		p, err := a.Service.Store.Page(id)
		if err != nil {
			return err
		}
		message := ""
		switch c.QueryParam("done") {
		case "saved":
			message = "Черновик сохранён. Публичная версия не изменилась."
		case "published":
			message = "Материал опубликован."
		case "scheduled":
			message = "Публикация выбранной версии запланирована. Позднейшие правки черновика не изменят запланированный снимок."
		case "restored":
			message = "Версия восстановлена в черновик. Публичная версия не изменилась."
		}
		return a.editPage(c, p, 200, message)
	})
	g.POST("/pages/save", func(c *echo.Context) error {
		p, err := postedPage(c)
		if err != nil {
			return err
		}
		intent := c.FormValue("intent")
		if intent != "save" && intent != "publish" && intent != "autosave" {
			return service.ErrInvalid
		}
		if intent == "autosave" && p.ID == 0 {
			if strings.TrimSpace(p.Title) == "" {
				p.Title = "Без названия"
			}
			if p.Slug == "" {
				p.Slug = "draft-" + service.Token()[:12]
			}
		}

		if err = a.checkPageTemplate(p); err != nil {
			return err
		}
		saved, err := a.Service.Save(c.Request().Context(), p, intent == "publish")
		if err != nil {
			status, message := http.StatusBadRequest, "Проверьте заголовок, адрес, изображение и содержимое страницы."
			switch {
			case errors.Is(err, store.ErrConflict):
				status, message = 409, "Страница изменена в другом окне. Ваши правки остались здесь. Откройте актуальную версию в новой вкладке и сравните изменения."
			case strings.Contains(err.Error(), "constraint failed"):
				status, message = 409, "Этот адрес уже занят. Выберите другой адрес страницы."
			case errors.Is(err, service.ErrInvalid):
			default:
				return err
			}
			if intent == "autosave" {
				return c.JSON(status, map[string]string{"message": message})
			}
			return a.editPage(c, p, status, message)
		}
		if intent == "autosave" {
			return c.JSON(200, map[string]any{"id": saved.ID, "version": saved.Version, "title": saved.Title, "slug": pageLeaf(saved.Slug), "edit_url": "/admin/pages/" + strconv.FormatInt(saved.ID, 10) + "/edit"})
		}
		done := "saved"
		if intent == "publish" {
			done = "published"
		}
		return c.Redirect(303, "/admin/pages/"+strconv.FormatInt(saved.ID, 10)+"/edit?done="+done)
	}, a.csrf)
	g.GET("/media/choices", func(c *echo.Context) error {
		page, err := mediaPageNumber(c)
		if err != nil {
			return err
		}
		selected := c.QueryParam("selected")
		if c.QueryParams().Has("cover") {
			selected = c.QueryParam("cover")
		}
		files, total, err := a.mediaSelection(c.QueryParam("q"), page, selected, mediaOwner(c.Request().Context()))
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, MediaPicker(files, total, page, c.QueryParam("q"), selected))
	})
}

func contentLabel(kind, page, post string) string {
	if kind == "post" {
		return post
	}
	return page
}

func navAttributes(active bool, label string) templ.Attributes {
	attrs := templ.Attributes{"title": label, "aria-label": label}
	if active {
		attrs["aria-current"] = "page"
	}
	return attrs
}

func revisionID(c *echo.Context) (int64, int64, error) {
	id, err := pageID(c)
	if err != nil {
		return 0, 0, err
	}
	version, err := strconv.ParseInt(c.Param("revision"), 10, 64)
	if err != nil || version < 1 {
		return 0, 0, service.ErrInvalid
	}
	return id, version, nil
}

func themeLabel(theme string) string {
	if strings.HasPrefix(theme, "theme:") {
		return strings.TrimPrefix(theme, "theme:")
	}
	switch theme {
	case "minimal":
		return "Минимал"
	case "custom":
		return "Локальный шаблон"
	case "unavailable":
		return "Недоступна"
	default:
		return "Журнал"
	}
}
func tableContentDate(at int64, zone ...string) string {
	if len(zone) == 0 {
		return contentDate(at)
	}
	if at == 0 {
		return "—"
	}
	location, err := time.LoadLocation(zone[0])
	if err != nil {
		location = time.UTC
	}
	return time.Unix(at, 0).In(location).Format("02.01.2006 15:04")
}
func storageSize(size int64) string {
	if size >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(size)/(1<<30))
	}
	if size >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	}
	return fmt.Sprintf("%.0f KiB", float64(size)/1024)
}

type sidebarCollapsedKey struct{}

func sidebarCollapsed(ctx context.Context) bool {
	collapsed, _ := ctx.Value(sidebarCollapsedKey{}).(bool)
	return collapsed
}
