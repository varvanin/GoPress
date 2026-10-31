package httpui

import (
	"errors"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

func currentSessionReference(c *echo.Context) string {
	cookie, err := c.Cookie("session")
	if err != nil {
		return ""
	}
	return service.Hash(cookie.Value)
}
func (a *App) securityPage(c *echo.Context, status int, message string) error {
	user, _ := store.Actor(c.Request().Context())
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	factor, err := a.Service.SecondFactorStatus(c.Request().Context())
	if err != nil {
		return err
	}
	sessions, total, err := a.Service.Store.UserSessions(user.ID, currentSessionReference(c))
	if err != nil {
		return err
	}
	return htmlComponent(c, status, AccountSecurity(site.Title, a.csrfToken(c), site.Timezone, user, factor, sessions, total, message))
}
func (a *App) credentialDownload(c *echo.Context, name, text string, logout bool) error {
	if logout {
		a.cookie(c, "session", "", -1)
		a.cookie(c, "csrf", "", -1)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	return c.Blob(200, "text/plain; charset=utf-8", []byte(text))
}
func (a *App) securityRoutes(g *echo.Group) {
	g.GET("/account/security", func(c *echo.Context) error { return a.securityPage(c, 200, "") })
	g.POST("/account/2fa/setup", func(c *echo.Context) error {
		value, err := a.Service.BeginSecondFactor(c.Request().Context(), c.FormValue("password"))
		if err != nil {
			return a.securityError(c, err)
		}
		text := i18n.T(c.Request().Context(), "Ключ настройки 2FA GoPress") + "\n\n" + value.Secret + "\n\n" + value.URI + "\n"
		return a.credentialDownload(c, "gopress-authenticator-key.txt", text, false)
	}, a.csrf)
	g.POST("/account/2fa/enable", func(c *echo.Context) error {
		codes, err := a.Service.EnableSecondFactor(c.Request().Context(), c.FormValue("password"), c.FormValue("code"))
		if err != nil {
			return a.securityError(c, err)
		}
		return a.recoveryDownload(c, codes)
	}, a.csrf)
	g.POST("/account/2fa/change", func(c *echo.Context) error {
		action := c.FormValue("action")
		if action != "disable" && action != "rotate" {
			return service.ErrInvalid
		}
		codes, err := a.Service.ChangeSecondFactor(c.Request().Context(), c.FormValue("password"), c.FormValue("code"), action == "disable")
		if err != nil {
			return a.securityError(c, err)
		}
		if action == "rotate" {
			return a.recoveryDownload(c, codes)
		}
		a.cookie(c, "session", "", -1)
		a.cookie(c, "csrf", "", -1)
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.Redirect(303, "/login")
	}, a.csrf)
	g.POST("/account/sessions/revoke", func(c *echo.Context) error {
		current := currentSessionReference(c)
		reference := c.FormValue("reference")
		var err error
		if c.FormValue("others") == "true" {
			err = a.Service.RevokeOtherSessions(c.Request().Context(), current)
		} else {
			err = a.Service.RevokeSession(c.Request().Context(), current, reference)
		}
		if errors.Is(err, store.ErrConflict) {
			return a.securityPage(c, 409, "Сеанс уже завершён или недоступен.")
		}
		if err != nil {
			return err
		}
		if reference == current {
			a.cookie(c, "session", "", -1)
			a.cookie(c, "csrf", "", -1)
			c.Response().Header().Set("HX-Redirect", "/login")
			return c.Redirect(303, "/login")
		}
		return c.Redirect(303, "/admin/account/security")
	}, a.csrf)
}
func (a *App) recoveryDownload(c *echo.Context, codes []string) error {
	text := i18n.T(c.Request().Context(), "Резервные коды GoPress") + "\n\n" + strings.Join(codes, "\n") + "\n\n" + i18n.T(c.Request().Context(), "Каждый код используется один раз. Сохраните файл отдельно от сайта. Войдите заново с новым кодом.") + "\n"
	return a.credentialDownload(c, "gopress-recovery-codes.txt", text, true)
}
func (a *App) securityError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrUnauthorized), errors.Is(err, service.ErrInvalid):
		return a.securityPage(c, 400, "Проверьте пароль и новый код. Сначала получите ключ настройки.")
	case errors.Is(err, store.ErrConflict):
		return a.securityPage(c, 409, "Настройки защиты изменились или код уже использован. Обновите страницу и используйте новый код.")
	case errors.Is(err, service.ErrSecondFactorUnavailable):
		return a.securityPage(c, 400, "Ключ 2FA недоступен. Используйте резервный код или восстановление на сервере.")
	default:
		return err
	}
}
