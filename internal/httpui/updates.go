package httpui

import (
	"errors"
	"os"
	"time"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"gopress/internal/update"
)

func updatePhase(phase string) string {
	switch phase {
	case "checking":
		return "Verifying the release and Dokploy connection."
	case "backing-up":
		return "Creating and verifying a backup."
	case "deploying":
		return "Sending the new image to Dokploy."
	case "requested":
		return "Dokploy accepted the request. Waiting for the new version to start."
	case "complete":
		return "Update installed and verified."
	case "failed":
		return "The update did not start. Check the connection and backups before retrying."
	case "uncertain":
		return "The deployment outcome is unconfirmed. Maintenance remains enabled. Check the deployment in Dokploy."
	}
	return ""
}
func (a *App) updatesPage(c *echo.Context, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	state, err := a.Service.UpdateState()
	if err != nil {
		return err
	}
	release, err := a.Service.CheckedRelease()
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	} else if err != nil {
		message = "Could not verify release metadata. Check for updates again."
	}
	return htmlComponent(c, status, UpdatesPage(site.Title, a.csrfToken(c), release, state, service.UpdatesConfigured(), message, status >= 400))
}
func (a *App) updateRoutes(g *echo.Group) {
	g.GET("/updates", func(c *echo.Context) error { return a.updatesPage(c, 200, "") })
	g.GET("/updates/status", func(c *echo.Context) error {
		state, err := a.Service.UpdateState()
		if err != nil {
			return err
		}
		if state.Phase == "complete" || state.Phase == "failed" {
			c.Response().Header().Set("HX-Refresh", "true")
		}
		return htmlComponent(c, 200, UpdateStatus(state))
	})
	g.POST("/updates/check", func(c *echo.Context) error {
		err := a.Service.CheckUpdates(c.Request().Context())
		if errors.Is(err, store.ErrConflict) {
			return a.updatesPage(c, 409, "An update is already running. Wait for it to finish.")
		}
		if err != nil {
			return a.updatesPage(c, 400, "Could not check for updates. Check the release source and signing key.")
		}
		return a.updatesPage(c, 200, "Update check completed.")
	}, a.csrf)
	g.POST("/updates/install", func(c *echo.Context) error {
		err := a.Service.StartUpdate(c.Request().Context(), c.FormValue("version"))
		if errors.Is(err, store.ErrConflict) {
			return a.updatesPage(c, 409, "An update is already running. Wait for it to finish.")
		}
		if err != nil {
			return a.updatesPage(c, 400, "Could not start the update. Check Dokploy connectivity and the release version.")
		}
		return c.Redirect(303, "/admin/updates")
	}, a.csrf)
}
func updatePolling(state service.UpdateState) bool {
	elapsed := time.Now().Unix() - state.Started
	return state.Pending() && state.Phase != "uncertain" && elapsed >= 0 && elapsed < 300
}
func updateAvailable(release update.Release) bool { return service.CanInstallRelease(release) }
