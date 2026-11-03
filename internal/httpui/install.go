package httpui

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

type installationForm struct {
	Title, BaseURL, Timezone, Email, Language, Message string
	Unlocked                                           bool
}

func (a *App) unlockInstallation(c *echo.Context) error {
	if a.Service.Installed() {
		return store.ErrInstalled
	}
	token := c.FormValue("token")
	if a.Service.InstallToken == "" || !service.Equal(token, a.Service.InstallToken) {
		return service.ErrUnauthorized
	}
	// The site URL is not saved yet, so use the request scheme for this temporary grant.
	c.SetCookie(&http.Cookie{Name: "installation", Value: token, Path: "/", MaxAge: 1800, HttpOnly: true, Secure: c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode})
	return c.NoContent(http.StatusNoContent)
}

func (a *App) installationToken(c *echo.Context) string {
	if c.Request().Method == http.MethodPost {
		if token := c.FormValue("token"); token != "" {
			return token
		}
	}
	if cookie, err := c.Cookie("installation"); err == nil {
		return cookie.Value
	}
	return ""
}

func (a *App) renderInstallation(c *echo.Context, status int, value installationForm) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	if !i18n.Supported(value.Language) {
		value.Language = "en"
	}
	c.SetRequest(c.Request().WithContext(i18n.WithLanguage(c.Request().Context(), value.Language)))
	csrf := a.csrfToken(c)
	c.Response().WriteHeader(status)
	return Installation(csrf, value).Render(c.Request().Context(), c.Response())
}

func (a *App) install(c *echo.Context) error {
	if a.Service.Installed() {
		return store.ErrInstalled
	}
	token := a.installationToken(c)
	value := installationForm{Title: c.FormValue("title"), BaseURL: c.FormValue("base_url"), Timezone: c.FormValue("timezone"), Email: c.FormValue("email"), Language: c.FormValue("language"), Unlocked: true}
	if value.Language == "" {
		value.Language = "ru"
	}
	err := a.Service.Install(c.Request().Context(), token, store.Site{Title: value.Title, BaseURL: value.BaseURL, Timezone: value.Timezone}, value.Email, c.FormValue("password"), value.Language)
	if errors.Is(err, service.ErrInvalid) {
		value.Message = "Check the site details, email and password. Use a password of 12–72 bytes."
		return a.renderInstallation(c, http.StatusBadRequest, value)
	}
	if err != nil {
		return err
	}
	a.cookie(c, "installation", "", -1)
	return c.Redirect(http.StatusSeeOther, "/login")
}

func (a *App) installationTranslations(c *echo.Context) error {
	if a.Service.Installed() {
		return store.ErrInstalled
	}
	language := c.QueryParam("language")
	if !i18n.Supported(language) {
		return service.ErrInvalid
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, i18n.Catalog(language))
}
