package httpui

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/backup"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

func (a *App) operationGuard(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		path := c.Request().URL.Path
		exclusive := c.Request().Method == "POST" && (path == "/admin/tools/backups/create" || path == "/admin/tools/backups/restore" || path == "/admin/tools/backups/delete" || path == "/admin/tools/backups/import")
		if exclusive {
			a.Service.Operations.Lock()
			defer a.Service.Operations.Unlock()
		} else {
			a.Service.Operations.RLock()
			defer a.Service.Operations.RUnlock()
		}
		active, err := a.Service.Maintenance()
		if err != nil {
			return err
		}
		public := !isSystemPath(path)
		if active && (public || c.Request().Method != "GET" && c.Request().Method != "HEAD" && !allowedMaintenanceWrite(path)) {
			c.Response().Header().Set("Retry-After", "300")
			c.Response().Header().Set("Cache-Control", "no-store")
			c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
			if public && (c.Request().Method == http.MethodGet || c.Request().Method == http.MethodHead || strings.Contains(c.Request().Header.Get("Accept"), "text/html")) {
				c.Response().Header().Set("X-Content-Type-Options", "nosniff")
				c.Response().Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
				c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
				if c.Request().Method == http.MethodHead {
					return c.NoContent(503)
				}
				ctx := c.Request().Context()
				if settings, err := a.Service.Store.Settings(); err == nil {
					ctx = i18n.WithLanguage(ctx, settings.Language)
				}
				c.SetRequest(c.Request().WithContext(ctx))
				retry := "/" + strings.TrimLeft(c.Request().URL.EscapedPath(), "/")
				return htmlComponent(c, 503, MaintenancePage(retry))
			}
			return echo.NewHTTPError(503, "Site maintenance in progress")
		}
		return next(c)
	}
}
func isSystemPath(path string) bool {
	return path == "/login" || path == "/logout" || path == "/forgot-password" || path == "/reset-password" || path == "/healthz" || path == "/readyz" || path == "/install" || len(path) >= 7 && path[:7] == "/admin/" || path == "/admin" || len(path) >= 8 && path[:8] == "/assets/"
}
func allowedMaintenanceWrite(path string) bool {
	return len(path) >= 13 && path[:13] == "/admin/tools/" || path == "/login" || path == "/logout" || path == "/forgot-password" || path == "/reset-password"
}
func (a *App) toolsPage(c *echo.Context, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	active, err := a.Service.Maintenance()
	if err != nil {
		return err
	}
	tab := c.QueryParam("tab")
	if tab != "system" && tab != "audit" {
		tab = "backups"
	}
	var backups []service.BackupInfo
	if tab == "backups" {
		backups, err = a.Service.Backups()
	}

	if err != nil {
		return err
	}
	var diagnostics service.Diagnostics
	if tab == "system" {
		diagnostics, err = a.Service.Diagnose()
	}
	if err != nil {
		return err
	}
	page, err := pageNumber(c)
	if err != nil {
		return err
	}
	var events []store.AuditEvent
	total := 0
	if tab == "audit" {
		events, total, err = a.Service.Store.AuditEvents(page)
	}
	if err != nil {
		return err
	}
	settings, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, Tools(site.Title, a.csrfToken(c), tab, active, backups, diagnostics, settings, events, total, page, message, status >= 400))
}
func (a *App) toolsRoutes(g *echo.Group) {
	g.GET("/tools", func(c *echo.Context) error { return a.toolsPage(c, 200, "") })
	g.POST("/tools/retention", func(c *echo.Context) error {
		days, err := strconv.Atoi(c.FormValue("days"))
		if err != nil || days < 1 || days > 3650 {
			return service.ErrInvalid
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		settings, err := a.Service.Store.Settings()
		if err != nil {
			return err
		}
		settings.Version = version
		settings.AuditRetentionDays = days
		if err = a.Service.SaveSettings(c.Request().Context(), settings); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/tools?tab=audit")
	}, a.csrf)
	g.POST("/tools/maintenance", func(c *echo.Context) error {
		if c.FormValue("enabled") != "true" && c.FormValue("enabled") != "false" {
			return service.ErrInvalid
		}
		if err := a.Service.SetMaintenance(c.FormValue("enabled") == "true"); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/tools?tab=system")
	}, a.csrf)
	g.POST("/tools/backups/create", func(c *echo.Context) error {
		if _, err := a.Service.CreateBackup(); err != nil {
			return err
		}
		return a.toolsPage(c, 200, "Бэкап создан и проверен. Скачайте копию на отдельный носитель.")
	}, a.csrf)
	g.POST("/tools/backups/verify", func(c *echo.Context) error {
		path, err := a.Service.BackupPath(c.FormValue("backup"))
		if err != nil {
			return err
		}
		if err = backup.Verify(path); err != nil {
			return a.toolsPage(c, 400, "Бэкап не прошёл проверку. Восстановление не выполнялось.")
		}
		return a.toolsPage(c, 200, "Контрольные суммы и база бэкапа проверены.")
	}, a.csrf)
	g.POST("/tools/backups/restore", func(c *echo.Context) error {
		if c.FormValue("confirm") != "RESTORE" {
			return service.ErrInvalid
		}
		active, err := a.Service.Maintenance()
		if err != nil {
			return err
		}
		if !active {
			return a.toolsPage(c, 400, "Для восстановления сначала включите режим обслуживания.")
		}
		if err = a.Service.RestoreBackup(c.FormValue("backup")); err != nil {
			return err
		}
		a.cookie(c, "session", "", -1)
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.Redirect(303, "/login")
	}, a.csrf)
	g.POST("/tools/backups/delete", func(c *echo.Context) error {
		path, err := a.Service.BackupPath(c.FormValue("backup"))
		if err != nil {
			return err
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/tools")
	}, a.csrf)
	g.GET("/tools/backups/:backup/download", func(c *echo.Context) error {
		path, err := a.Service.BackupPath(c.Param("backup"))
		if err != nil {
			return err
		}
		if err = backup.Verify(path); err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", `attachment; filename="`+c.Param("backup")+`.zip"`)
		c.Response().Header().Set("Content-Type", "application/zip")
		return backup.WriteZIP(c.Response(), path)
	})
	g.POST("/tools/backups/import", func(c *echo.Context) error {
		if err := c.Request().ParseMultipartForm(1 << 20); err != nil {
			return service.ErrInvalid
		}
		defer c.Request().MultipartForm.RemoveAll()
		file, header, err := c.Request().FormFile("backup_file")
		if err != nil {
			return service.ErrInvalid
		}
		defer file.Close()
		root := filepath.Join(a.Service.Store.Dir, "backups")
		if err = os.MkdirAll(root, 0700); err != nil {
			return err
		}
		id, err := a.Service.ImportBackup(file, header.Size)
		if errors.Is(err, service.ErrInvalid) {
			return a.toolsPage(c, 400, "Некорректный архив бэкапа.")
		}
		if err != nil {
			return err
		}
		return a.toolsPage(c, 200, i18n.T(c.Request().Context(), "Бэкап импортирован: ")+id)
	}, a.csrf)
}
