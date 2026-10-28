package httpui

import (
	"context"
	"errors"
	"strconv"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
	selectcomp "gopress/internal/ui/components/select"
)

func settingsTab(value string) string {
	switch value {
	case "home", "mail", "language":
		return value
	}
	return "general"
}
func homePageItems(ctx context.Context, pages []store.Page, selected int64) []selectcomp.ItemData {
	items := []selectcomp.ItemData{{Value: "0", Label: i18n.T(ctx, "Выберите опубликованную страницу")}}
	found := selected == 0
	for _, p := range pages {
		items = append(items, selectcomp.ItemData{Value: strconv.FormatInt(p.ID, 10), Label: p.Title})
		found = found || p.ID == selected
	}
	if !found {
		items = append(items, selectcomp.ItemData{Value: strconv.FormatInt(selected, 10), Label: i18n.T(ctx, "Выбранная страница недоступна")})
	}
	return items
}
func (a *App) settingsPage(c *echo.Context, tab string, status int, message string, posted *store.Settings, postedMail ...*service.MailSettings) error {
	value, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	site := value.Site
	if posted != nil {
		value = *posted
	}
	mail, err := a.Service.MailSettings()
	if err != nil {
		return err
	}
	if len(postedMail) > 0 {
		value := *postedMail[0]
		value.Password = mail.Password
		value.Environment = mail.Environment
		mail = value
	}
	pages, err := a.Service.Store.HomePages()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, SettingsPage(site.Title, a.csrfToken(c), settingsTab(tab), value, mail, pages, message, status >= 400, posted != nil || len(postedMail) > 0))
}
func settingsRedirect(c *echo.Context, tab string) error {
	path := "/admin/settings?tab=" + tab + "&saved=1"
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Trigger", "settingsSaved")
		c.Response().Header().Set("HX-Redirect", path)
		return c.NoContent(204)
	}
	return c.Redirect(303, path)
}
func (a *App) settingsRoutes(g *echo.Group) {
	g.GET("/settings", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Настройки сохранены."
		}
		return a.settingsPage(c, settingsTab(c.QueryParam("tab")), 200, message, nil)
	})
	g.POST("/settings/:section", func(c *echo.Context) error {
		section := c.Param("section")
		if section == "mail-test" {
			err := a.Service.SendTestMail(c.Request().Context(), c.FormValue("test_recipient"))
			if errors.Is(err, service.ErrInvalid) {
				return a.settingsPage(c, "mail", 400, "Укажите корректный email для тестового письма.", nil)
			}
			if err != nil {
				return a.settingsPage(c, "mail", 400, "Не удалось отправить письмо. Проверьте SMTP, адрес отправителя и доступ к почтовому серверу.", nil)
			}
			return a.settingsPage(c, "mail", 200, "Тестовое письмо отправлено.", nil)
		}
		if section == "mail" {
			value := service.MailSettings{Version: c.FormValue("mail_version"), Enabled: c.FormValue("enabled") == "true", Host: c.FormValue("host"), Port: c.FormValue("port"), From: c.FormValue("from"), Username: c.FormValue("username"), Password: c.FormValue("password")}
			err := a.Service.SaveMailSettings(value, c.FormValue("clear_password") == "1")
			if errors.Is(err, store.ErrConflict) {
				return a.settingsPage(c, "mail", 409, "Настройки почты изменены в другой вкладке. Откройте актуальную версию.", nil, &value)
			}
			if errors.Is(err, service.ErrInvalid) {
				return a.settingsPage(c, "mail", 400, "Проверьте SMTP-сервер, порт и адрес отправителя. Настройки из окружения меняются на сервере.", nil, &value)
			}
			if err != nil {
				return err
			}
			return settingsRedirect(c, "mail")
		}
		if section != "general" && section != "home" && section != "language" {
			return service.ErrInvalid
		}
		value, err := a.Service.Store.Settings()
		if err != nil {
			return err
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil || version < 0 {
			return service.ErrInvalid
		}
		value.Version = version
		switch section {
		case "general":
			value.Site.Title = c.FormValue("title")
			value.Site.BaseURL = c.FormValue("base_url")
			value.Site.Timezone = c.FormValue("timezone")
			value.Description = c.FormValue("description")
		case "home":
			value.HomeMode = c.FormValue("home_mode")
			value.HomePageID, err = strconv.ParseInt(c.FormValue("home_page_id"), 10, 64)
			if err != nil {
				return service.ErrInvalid
			}
			value.PostsPerPage, err = strconv.Atoi(c.FormValue("posts_per_page"))
			if err != nil {
				return service.ErrInvalid
			}
		case "language":
			value.Language = c.FormValue("language")
		}
		err = a.Service.SaveSettings(c.Request().Context(), value)
		if errors.Is(err, store.ErrConflict) {
			return a.settingsPage(c, section, 409, "Настройки изменены в другой вкладке или выбранная страница больше не опубликована. Проверьте актуальные данные.", &value)
		}
		if errors.Is(err, service.ErrInvalid) {
			return a.settingsPage(c, section, 400, "Проверьте заполненные поля: название, адрес сайта, часовой пояс и допустимые значения настроек.", &value)
		}
		if err != nil {
			return err
		}
		return settingsRedirect(c, section)
	}, a.csrf)
}
