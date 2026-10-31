package httpui

import (
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"strconv"
	"strings"
)

func (a *App) redirectsPage(c *echo.Context, status int, message, slug string, id int64) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	redirects, err := a.Service.Store.ManualRedirects()
	if err != nil {
		return err
	}
	missing, err := a.Service.Store.MissingPaths()
	if err != nil {
		return err
	}
	pages, err := a.Service.Store.RedirectTargets()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, RedirectsPage(site.Title, a.csrfToken(c), site.Timezone, redirects, missing, pages, message, slug, id))
}
func (a *App) redirectRoutes(g *echo.Group) {
	g.GET("/seo/redirects", func(c *echo.Context) error { return a.redirectsPage(c, 200, "", c.QueryParam("from"), 0) })
	g.POST("/seo/redirects", func(c *echo.Context) error {
		slug := strings.TrimPrefix(strings.TrimSpace(c.FormValue("slug")), "/")
		id, err := strconv.ParseInt(c.FormValue("page_id"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		if c.FormValue("action") == "delete" {
			err = a.Service.Store.DeleteManualRedirect(c.Request().Context(), slug, id)
		} else {
			err = a.Service.SaveRedirect(c.Request().Context(), slug, id)
		}
		if errors.Is(err, store.ErrConflict) || errors.Is(err, service.ErrInvalid) {
			return a.redirectsPage(c, 409, "Проверьте адрес: он уже занят или цель недоступна.", slug, id)
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, "/admin/seo/redirects")
	}, a.csrf)
	g.POST("/seo/missing/clear", func(c *echo.Context) error {
		if err := a.Service.Store.ClearMissingPaths(c.Request().Context()); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/seo/redirects")
	}, a.csrf)
}
