package httpui

import (
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"io"
)

func portableUpload(c *echo.Context) ([]byte, error) {
	header, err := c.FormFile("content")
	if err != nil || header.Size > service.MaxPortableContent {
		return nil, service.ErrInvalid
	}
	f, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, service.MaxPortableContent+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > service.MaxPortableContent {
		return nil, service.ErrInvalid
	}
	return raw, nil
}
func (a *App) portableContentRoutes(g *echo.Group) {
	g.GET("/tools/content", func(c *echo.Context) error {
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, PortableContentPage(site.Title, a.csrfToken(c)))
	})
	g.POST("/tools/content/inspect", func(c *echo.Context) error {
		raw, err := portableUpload(c)
		if err != nil {
			return err
		}
		bundle, err := service.InspectContent(raw)
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, PortableContentReview(bundle, service.Hash(string(raw)), "import-"+service.Token()[:8], ""))
	}, a.csrf)
	g.POST("/tools/content/import", func(c *echo.Context) error {
		raw, err := portableUpload(c)
		if err != nil {
			return err
		}
		bundle, err := service.InspectContent(raw)
		if err != nil {
			return err
		}
		if service.Hash(string(raw)) != c.FormValue("digest") {
			return service.ErrInvalid
		}
		_, err = a.Service.ImportContent(c.Request().Context(), raw, c.FormValue("prefix"))
		if errors.Is(err, store.ErrConflict) || errors.Is(err, service.ErrInvalid) {
			return htmlComponent(c, 409, PortableContentReview(bundle, c.FormValue("digest"), c.FormValue("prefix"), "Импорт отменён: проверьте префикс, изображения и содержимое. Материалы не добавлены."))
		}
		if err != nil {
			return err
		}
		c.Response().Header().Set("HX-Redirect", "/admin/pages")
		return c.NoContent(200)
	}, a.csrf)
}
