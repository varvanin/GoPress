package httpui

import (
	"context"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func adminRole(ctx context.Context) string {
	if u, ok := store.Actor(ctx); ok {
		return u.Role
	}
	return "admin"
}
func mediaOwner(ctx context.Context) int64 {
	if u, ok := store.Actor(ctx); ok && u.Role == "author" {
		return u.ID
	}
	return 0
}
func (a *App) permissions(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		u, _ := store.Actor(ctx)
		path := c.Path()
		method := c.Request().Method
		if u.Role != "admin" {
			if path == "/admin" {
				return c.Redirect(303, "/admin/posts")
			}
			allowed := strings.HasPrefix(path, "/admin/pages") || strings.HasPrefix(path, "/admin/posts") || strings.HasPrefix(path, "/admin/media") || strings.HasPrefix(path, "/admin/account") || path == "/admin/csrf" || path == "/admin/translations.js" || path == "/admin/content/bulk"
			if !allowed {
				return service.ErrForbidden
			}
			if u.Role == "author" {
				if strings.HasPrefix(path, "/admin/posts/terms") || path == "/admin/pages/new" || path == "/admin/pages" && method == "GET" || strings.HasSuffix(path, "/publish") || strings.HasSuffix(path, "/schedule") {
					return service.ErrForbidden
				}
				if raw := c.Param("id"); raw != "" {
					id, err := strconv.ParseInt(raw, 10, 64)
					if err != nil {
						return service.ErrInvalid
					}
					p, err := a.Service.Store.Page(id)
					if err != nil {
						return err
					}
					if !service.CanEdit(ctx, p) {
						return service.ErrForbidden
					}
					if strings.HasSuffix(path, "/state") {
						action := c.FormValue("action")
						if action == "unpublish" || action == "cancel_schedule" || p.Published && (action == "trash" || action == "delete") {
							return service.ErrForbidden
						}
					}
				}
				if path == "/admin/content/bulk" {
					if err := c.Request().ParseForm(); err != nil {
						return service.ErrInvalid
					}
					action := c.FormValue("action")
					for _, raw := range c.Request().PostForm["selection"] {
						id, err := strconv.ParseInt(strings.Split(raw, ":")[0], 10, 64)
						if err != nil {
							return service.ErrInvalid
						}
						p, err := a.Service.Store.Page(id)
						if err != nil {
							return err
						}
						if !service.CanEdit(ctx, p) || p.Published && (action == "trash" || action == "delete") {
							return service.ErrForbidden
						}
					}
				}
				if strings.HasPrefix(path, "/admin/media") {
					name := c.QueryParam("file")
					if method != "GET" {
						name = c.FormValue("name")
					}
					if name != "" {
						owner, err := a.Service.Store.MediaOwner(name)
						if err != nil {
							return err
						}
						if owner != u.ID {
							return service.ErrForbidden
						}
					}
				}
			}
		}
		err := next(c)
		response, _ := echo.UnwrapResponse(c.Response())
		contentLogged := path == "/admin/pages" || path == "/admin/pages/:id" || path == "/admin/pages/save" || path == "/admin/pages/:id/publish" || path == "/admin/pages/:id/state" || path == "/admin/content/bulk" || path == "/admin/users/save" || path == "/admin/account/password" || path == "/admin/forms/:form/entries/delete"
		if err == nil && !contentLogged && method != "GET" && method != "HEAD" && response != nil && response.Status < 400 {
			object := c.Param("id")
			if object == "" {
				object = c.Param("form")
				if object == "" {
					object = c.Param("module")
				}
			}
			if object == "" && strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				for _, key := range []string{"id", "backup", "theme", "name", "position"} {
					if value := c.FormValue(key); value != "" {
						object = value
						break
					}
				}
			}
			if auditErr := a.Service.Store.Audit(ctx, method+" "+path, object); auditErr != nil {
				return auditErr
			}
		}
		return err
	}
}

func allowNav(ctx context.Context, path string) bool {
	role := adminRole(ctx)
	if role == "admin" {
		return true
	}
	if path == "/admin/account" || path == "/admin/posts" || path == "/admin/media" {
		return true
	}
	return role == "editor" && (path == "/admin/pages" || path == "/admin/posts/terms")
}
func roleLabel(role string) string {
	switch role {
	case "admin":
		return "Администратор"
	case "editor":
		return "Редактор"
	}
	return "Автор"
}

func currentUser(ctx context.Context, id int64) bool {
	u, ok := store.Actor(ctx)
	return ok && u.ID == id
}
