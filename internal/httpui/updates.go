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
		return "Проверяем релиз и подключение к Dokploy."
	case "backing-up":
		return "Создаём и проверяем резервную копию."
	case "deploying":
		return "Передаём новый образ в Dokploy."
	case "requested":
		return "Dokploy принял запрос. Ожидаем запуск новой версии."
	case "complete":
		return "Обновление установлено и проверено."
	case "failed":
		return "Обновление не началось. Проверьте подключение и резервные копии перед повтором."
	case "uncertain":
		return "Результат установки не подтверждён. Сайт остаётся в обслуживании. Проверьте развёртывание в Dokploy."
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
		message = "Не удалось проверить сведения о релизе. Повторите проверку обновлений."
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
			return a.updatesPage(c, 409, "Обновление уже выполняется. Дождитесь завершения.")
		}
		if err != nil {
			return a.updatesPage(c, 400, "Не удалось проверить обновления. Проверьте источник релизов и ключ подписи.")
		}
		return a.updatesPage(c, 200, "Проверка обновлений завершена.")
	}, a.csrf)
	g.POST("/updates/install", func(c *echo.Context) error {
		err := a.Service.StartUpdate(c.Request().Context(), c.FormValue("version"))
		if errors.Is(err, store.ErrConflict) {
			return a.updatesPage(c, 409, "Обновление уже выполняется. Дождитесь завершения.")
		}
		if err != nil {
			return a.updatesPage(c, 400, "Не удалось начать обновление. Проверьте подключение Dokploy и актуальность релиза.")
		}
		return c.Redirect(303, "/admin/updates")
	}, a.csrf)
}
func updatePolling(state service.UpdateState) bool {
	elapsed := time.Now().Unix() - state.Started
	return state.Pending() && state.Phase != "uncertain" && elapsed >= 0 && elapsed < 300
}
func updateAvailable(release update.Release) bool { return service.CanInstallRelease(release) }
