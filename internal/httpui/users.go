package httpui

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

func pageNumber(c *echo.Context) (int, error) {
	page := 1
	if raw := c.QueryParam("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000000 {
			return 0, service.ErrInvalid
		}
		page = n
	}
	return page, nil
}
func (a *App) userPage(c *echo.Context, status int, u store.User, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, UserEditor(site.Title, a.csrfToken(c), u, message, status >= 400))
}
func (a *App) usersRoutes(g *echo.Group) {
	a.securityRoutes(g)
	g.GET("/users", func(c *echo.Context) error {
		page, err := pageNumber(c)
		if err != nil {
			return err
		}
		users, total, err := a.Service.Store.Users(page)
		if err != nil {
			return err
		}
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, Users(site.Title, a.csrfToken(c), users, total, page))
	})
	g.GET("/users/new", func(c *echo.Context) error { return a.userPage(c, 200, store.User{Role: "author"}, "") })
	g.GET("/users/:user/edit", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.Param("user"), 10, 64)
		if err != nil || id < 1 {
			return service.ErrInvalid
		}
		u, err := a.Service.Store.User(id)
		if err != nil {
			return err
		}
		return a.userPage(c, 200, u, "")
	})
	g.POST("/users/save", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.FormValue("id"), 10, 64)
		if err != nil || id < 0 {
			return service.ErrInvalid
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		u := store.User{ID: id, Version: version, Email: c.FormValue("email"), Name: c.FormValue("name"), Role: c.FormValue("role"), Blocked: c.FormValue("blocked") == "true"}
		_, err = a.Service.SaveUser(c.Request().Context(), u, c.FormValue("password"))
		if err != nil {
			message := "Проверьте email, имя, роль и пароль (12–72 байта)."
			code := 400
			if errors.Is(err, store.ErrLastAdmin) {
				message = "Нельзя заблокировать или понизить последнего администратора."
				code = 409
			} else if errors.Is(err, store.ErrConflict) || strings.Contains(err.Error(), "constraint failed") {
				message = "Пользователь изменён в другой вкладке или email уже занят."
				code = 409
			} else if !errors.Is(err, service.ErrInvalid) {
				return err
			}
			return a.userPage(c, code, u, message)
		}
		actor, _ := store.Actor(c.Request().Context())
		if u.ID == actor.ID {
			c.Response().Header().Set("HX-Trigger", "settingsSaved")
			c.Response().Header().Set("HX-Redirect", "/login")
			return c.Redirect(303, "/login")
		}
		return c.Redirect(303, "/admin/users")
	}, a.csrf)
	g.GET("/account", func(c *echo.Context) error {
		if u, ok := store.Actor(c.Request().Context()); ok && u.Role == "admin" {
			return c.Redirect(303, "/admin/users/"+strconv.FormatInt(u.ID, 10)+"/edit")
		}

		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		u, _ := store.Actor(c.Request().Context())
		return htmlComponent(c, 200, Account(site.Title, a.csrfToken(c), u, "", false))
	})
	g.POST("/account/password", func(c *echo.Context) error {
		if c.FormValue("password") != c.FormValue("password_confirm") {
			return service.ErrInvalid
		}
		if err := a.Service.ChangePassword(c.Request().Context(), c.FormValue("old_password"), c.FormValue("password")); err != nil {
			if !errors.Is(err, service.ErrInvalid) && !errors.Is(err, service.ErrUnauthorized) && !errors.Is(err, store.ErrConflict) {
				return err
			}
			site, e := a.Service.Store.Site()
			if e != nil {
				return e
			}
			u, _ := store.Actor(c.Request().Context())
			return htmlComponent(c, 400, Account(site.Title, a.csrfToken(c), u, "Проверьте текущий пароль. Новый пароль — от 12 до 72 байт.", true))
		}
		a.cookie(c, "session", "", -1)
		c.Response().Header().Set("HX-Trigger", "settingsSaved")
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.Redirect(303, "/login")
	}, a.csrf)
	a.Echo.GET("/forgot-password", func(c *echo.Context) error { return a.recoveryPage(c, false, "", 200) })
	a.Echo.POST("/forgot-password", func(c *echo.Context) error {
		err := a.Service.RequestPasswordReset(c.Request().Context(), c.RealIP(), c.FormValue("email"))
		if err != nil {
			return err
		}
		return a.recoveryPage(c, false, "Если активный аккаунт существует, письмо со ссылкой отправлено. Проверьте почту или обратитесь к администратору.", 200)
	}, a.csrf)
	a.Echo.GET("/reset-password", func(c *echo.Context) error { return a.recoveryPage(c, true, "", 200) })
	a.Echo.POST("/reset-password", func(c *echo.Context) error {
		if c.FormValue("password") != c.FormValue("password_confirm") {
			return a.recoveryPage(c, true, "Пароли не совпадают.", 400)
		}
		err := a.Service.ResetPassword(c.Request().Context(), c.RealIP(), c.FormValue("token"), c.FormValue("password"))
		if errors.Is(err, service.ErrInvalid) || errors.Is(err, sql.ErrNoRows) {
			return a.recoveryPage(c, true, "Ссылка недействительна или истекла. Запросите новую ссылку; пароль — от 12 до 72 байт.", 400)
		}
		if err != nil {
			return err
		}
		a.cookie(c, "session", "", -1)
		return c.Redirect(303, "/login")
	}, a.csrf)
}
func (a *App) recoveryPage(c *echo.Context, reset bool, message string, status int) error {
	settings, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	c.SetRequest(c.Request().WithContext(i18n.WithLanguage(c.Request().Context(), settings.Language)))
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
	c.Response().Header().Set("Referrer-Policy", "no-referrer")
	token := c.QueryParam("token")
	if c.Request().Method == "POST" {
		token = c.FormValue("token")
	}
	return htmlComponent(c, status, Recovery(a.csrfToken(c), token, message, reset, status >= 400))
}
