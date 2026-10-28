package httpui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

func (a *App) themeSource(mode string) (fs.FS, string, string, error) {
	if strings.HasPrefix(mode, "theme:") && service.ValidTheme(mode) {
		dir := filepath.Join(a.Service.Store.Dir, "theme", "packages", strings.TrimPrefix(mode, "theme:"))
		if _, err := os.Stat(filepath.Join(dir, "layout.html")); err != nil {
			return nil, "", "", err
		}
		return os.DirFS(dir), "", mode, nil
	}
	if mode == "custom" || mode == "" {
		dir := filepath.Join(a.Service.Store.Dir, "theme")
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() && legacyTheme(a.Service.Store.Dir) {
			return os.DirFS(dir), "", "custom", nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", "", err
		}
		if mode == "custom" {
			return nil, "", "", fmt.Errorf("local theme unavailable")
		}
	}
	if mode == "" {
		mode = "journal"
	}
	if mode == "minimal" {
		return resources, "theme-minimal/", mode, nil
	}
	if mode == "studio" {
		return resources, "theme-studio/", mode, nil
	}
	return resources, "theme/", mode, nil
}
func (a *App) validateTheme(appearance store.Appearance) error {
	source, prefix, _, err := a.themeSource(appearance.Theme)
	if err != nil {
		return err
	}
	return validateThemeFiles(source, prefix, appearance)
}
func (a *App) postedAppearance(c *echo.Context) (store.Appearance, error) {
	version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
	if err != nil {
		return store.Appearance{}, service.ErrInvalid
	}
	value := store.Appearance{Version: version, Theme: c.FormValue("theme"), HomeTitle: c.FormValue("home_title"), HomeDescription: c.FormValue("home_description"), SEO: store.SEO{Title: c.FormValue("seo_title"), Description: c.FormValue("seo_description"), NoIndex: c.FormValue("noindex") == "1"}}
	if c.FormValue("scope") == "theme" || (c.Path() == "/admin/themes" || c.Path() == "/admin/themes/preview") {
		current, e := a.Service.Store.Appearance()
		if e != nil {
			return value, e
		}
		value.SEO = current.SEO
		value.SEOOptions = current.SEOOptions
		if c.FormValue("intent") == "activate" {
			value.HomeTitle = current.HomeTitle
			value.HomeDescription = current.HomeDescription
		}
	}
	err = service.ValidateAppearance(&value)
	return value, err
}
func (a *App) appearancePage(c *echo.Context, status int, message string, _ *store.Appearance) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	value, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	_, _, resolved, resolveErr := a.themeSource(value.Theme)
	if resolveErr == nil && value.Theme == "" {
		value.Theme = resolved
	}
	themes, err := a.themes()
	if err != nil {
		return err
	}
	for i := range themes {
		if !strings.HasPrefix(themes[i].ID, "theme:") {
			themes[i].Name = i18n.T(c.Request().Context(), themes[i].Name)
			themes[i].Description = i18n.T(c.Request().Context(), themes[i].Description)
		}
	}
	return htmlComponent(c, status, AppearancePage(site.Title, a.csrfToken(c), value, themes, message, status >= 400))
}
func (a *App) appearanceRoutes(g *echo.Group) {
	a.themeRoutes(g)
	showThemes := func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Шаблон активирован."
		}
		if c.QueryParam("installed") == "1" {
			message = "Шаблон установлен. Его можно посмотреть и активировать."
		}
		if c.QueryParam("deleted") == "1" {
			message = "Шаблон удалён."
		}
		return a.appearancePage(c, 200, message, nil)
	}
	g.GET("/themes", showThemes)
	g.GET("/appearance", showThemes)
	saveTheme := func(c *echo.Context) error {
		a.themeMu.Lock()
		defer a.themeMu.Unlock()
		value, err := a.postedAppearance(c)
		if err != nil {
			return a.appearancePage(c, 400, "Проверьте шаблон, заголовок (до 200 символов) и описание главной (до 1000).", &value)
		}
		if err = a.validateTheme(value); err != nil {
			return a.appearancePage(c, 400, "Ошибка шаблона: "+err.Error(), &value)
		}
		if err = a.Service.SaveAppearance(c.Request().Context(), value); errors.Is(err, store.ErrConflict) {
			return a.appearancePage(c, 409, "Настройки изменены в другом окне. Обновите страницу перед сохранением.", &value)
		} else if err != nil {
			return err
		}
		if c.Request().Header.Get("HX-Request") == "true" {
			c.Response().Header().Set("HX-Location", `{"path":"/admin/themes?saved=1","target":"#main","select":"#main","swap":"outerHTML show:top"}`)
			return c.NoContent(204)
		}
		return c.Redirect(303, "/admin/themes?saved=1")
	}
	g.POST("/themes", saveTheme, a.csrf)
	g.POST("/appearance", saveTheme, a.csrf)
	g.GET("/seo", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "SEO-настройки сохранены."
		}
		return a.seoPage(c, 200, message, nil)
	})
	g.POST("/seo", func(c *echo.Context) error {
		value, e := a.postedSEOSettings(c)
		if e != nil {
			return e
		}
		e = a.Service.SaveSEOSettings(c.Request().Context(), value.Version, value.SEO, value.SEOOptions)
		if errors.Is(e, store.ErrConflict) {
			return a.seoPage(c, 409, "Настройки изменены в другом окне. Обновите страницу перед сохранением.", &value)
		}
		if errors.Is(e, service.ErrInvalid) {
			return a.seoPage(c, 400, "Проверьте длину текстов, переменные заголовков, URL и значения директив.", &value)
		}
		if e != nil {
			return e
		}
		return c.Redirect(303, "/admin/seo?tab="+seoTab(c)+"&saved=1")
	}, a.csrf)

	previewTheme := func(c *echo.Context) error {
		value, err := a.postedAppearance(c)
		if err != nil {
			return err
		}
		if err = a.validateTheme(value); err != nil {
			return echo.NewHTTPError(400, "Ошибка шаблона: "+err.Error())
		}
		return a.render(c, store.Page{Document: `{"type":"doc","content":[{"type":"paragraph"}]}`}, false, value)
	}
	g.POST("/themes/preview", previewTheme, a.csrf)
	g.POST("/appearance/preview", previewTheme, a.csrf)
}

func (a *App) seoPage(c *echo.Context, status int, message string, submitted *store.Appearance) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	value, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	if submitted != nil {
		value = *submitted
	}
	return htmlComponent(c, status, SEOPage(site.Title, a.csrfToken(c), value, site, seoTab(c), message, status >= 400))
}
