package httpui

import (
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"strconv"
	"strings"
)

func (a *App) reviewRoutes(g *echo.Group) {
	g.GET("/posts/review", func(c *echo.Context) error { return a.reviewsPage(c, 200, 0, "", "") })
	g.POST("/posts/review", func(c *echo.Context) error {
		id, e1 := strconv.ParseInt(c.FormValue("page_id"), 10, 64)
		version, e2 := strconv.ParseInt(c.FormValue("version"), 10, 64)
		action := c.FormValue("action")
		if e1 != nil || e2 != nil || id <= 0 || version <= 0 || (action != "publish" && action != "return") {
			return service.ErrInvalid
		}
		err := a.Service.ResolveReview(c.Request().Context(), id, version, action == "publish", strings.TrimSpace(c.FormValue("comment")))
		if errors.Is(err, store.ErrConflict) || errors.Is(err, service.ErrInvalid) {
			return a.reviewsPage(c, 409, id, c.FormValue("comment"), "Версия изменилась или комментарий пуст. Проверьте материал и повторите действие.")
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, "/admin/posts/review")
	}, a.csrf)
}

func (a *App) reviewsPage(c *echo.Context, status int, postedID int64, comment, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	owner := int64(0)
	if !service.CanPublish(c.Request().Context()) {
		owner = mediaOwner(c.Request().Context())
	}
	items, err := a.Service.Store.ContentReviews(owner)
	if err != nil {
		return err
	}
	return htmlComponent(c, status, ReviewsPage(site.Title, a.csrfToken(c), site.Timezone, items, postedID, comment, message))
}
