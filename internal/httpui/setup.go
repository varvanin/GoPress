package httpui

import (
	"database/sql"
	"errors"
	"github.com/labstack/echo/v5"
)

type SetupStep struct {
	Label, URL string
	Done       bool
}

func (a *App) setupPage(c *echo.Context) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	settings, err := a.Service.Store.Settings()
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
	_, _, _, templateErr := a.themeSource(appearance.Theme)
	homeReady := data.Posts.Published > 0
	if settings.HomeMode == "page" {
		_, err = a.Service.Store.PublicPage(settings.HomePageID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		homeReady = err == nil
	}
	backups, err := a.Service.Backups()
	if err != nil {
		return err
	}
	steps := []SetupStep{{"Site name and address", "/admin/settings", site.Title != "" && site.BaseURL != ""}, {"Available template selected", "/admin/themes", templateErr == nil}, {"First content published", "/admin/pages", data.Pages.Published+data.Posts.Published > 0}, {"Homepage has published content", "/admin/settings?tab=home", homeReady}, {"Backup exists", "/admin/tools", len(backups) > 0}}
	return htmlComponent(c, 200, SetupPage(site.Title, a.csrfToken(c), steps))
}
