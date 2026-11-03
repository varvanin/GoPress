package httpui

import (
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
	"net/http"
	"time"
)

func (a *App) aiPage(c *echo.Context, status int, message string, posted *service.AISettings) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	value, err := a.Service.AISettings()
	if err != nil {
		return err
	}
	configured := value.Key != ""
	if posted != nil {
		value.Model = posted.Model
	}
	value.Key = ""
	return htmlComponent(c, status, AIPage(site.Title, a.csrfToken(c), value, configured, message, status >= 400, posted != nil))
}
func (a *App) aiRoutes(g *echo.Group) {
	g.GET("/ai", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Settings saved."
		}
		return a.aiPage(c, 200, message, nil)
	})
	g.POST("/ai/settings", func(c *echo.Context) error {
		value := service.AISettings{Version: c.FormValue("version"), Key: c.FormValue("key"), Model: c.FormValue("model")}
		err := a.Service.SaveAISettings(c.Request().Context(), value, c.FormValue("clear_key") == "1")
		value.Key = ""
		if errors.Is(err, store.ErrConflict) {
			return a.aiPage(c, 409, "AI settings changed in another tab. Review the current values.", &value)
		}
		if errors.Is(err, service.ErrInvalid) {
			return a.aiPage(c, 400, "Check the API key and model ID.", &value)
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, "/admin/ai?saved=1")
	}, a.csrf)
	g.POST("/pages/ai", func(c *echo.Context) error {
		var in service.AIInput
		if err := decode(c, &in); err != nil {
			return err
		}
		// Generation can outlast the ordinary 30-second page response budget.
		if err := http.NewResponseController(c.Response()).SetWriteDeadline(time.Now().Add(90 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		text, err := a.Service.GenerateAI(c.Request().Context(), in)
		switch {
		case errors.Is(err, service.ErrAINotConfigured):
			return c.JSON(400, map[string]string{"error": i18n.T(c.Request().Context(), "Ask an administrator to configure OpenRouter under AI.")})
		case errors.Is(err, service.ErrRateLimit):
			return c.JSON(429, map[string]string{"error": i18n.T(c.Request().Context(), "AI is busy or the request limit has been reached. Try again later.")})
		case errors.Is(err, service.ErrAIUnavailable):
			return c.JSON(502, map[string]string{"error": i18n.T(c.Request().Context(), "OpenRouter returned no text. Check the key, balance and model, or try again later.")})
		case err != nil:
			return err
		}
		return c.JSON(200, map[string]string{"text": text})
	}, a.csrf)
}
