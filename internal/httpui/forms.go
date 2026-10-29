package httpui

import (
	"bytes"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

func formPath(id int64) string       { return "/admin/forms/" + strconv.FormatInt(id, 10) }
func formCode(id int64) string       { return "[form id=" + strconv.FormatInt(id, 10) + "]" }
func formFieldName(index int) string { return "field_" + strconv.Itoa(index) }
func (a *App) formsPage(c *echo.Context) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	forms, err := a.Service.Store.Forms()
	if err != nil {
		return err
	}
	mail, err := a.Service.MailSettings()
	if err != nil {
		return err
	}
	settings, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	return htmlComponent(c, 200, Forms(site.Title, a.csrfToken(c), forms, mail.Configured(), settings))
}
func (a *App) formEditor(c *echo.Context, status int, f store.Form, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, FormEditor(site.Title, a.csrfToken(c), f, message))
}
func formID(c *echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("form"), 10, 64)
	if err != nil || id < 1 {
		return 0, service.ErrInvalid
	}
	return id, nil
}
func postedForm(c *echo.Context) (store.Form, error) {
	if err := c.Request().ParseForm(); err != nil {
		return store.Form{}, service.ErrInvalid
	}
	f := store.Form{Name: c.FormValue("name"), Recipient: c.FormValue("recipient"), Button: c.FormValue("button"), Success: c.FormValue("success"), Enabled: c.FormValue("enabled") == "true"}
	var err error
	f.ID, err = strconv.ParseInt(c.FormValue("id"), 10, 64)
	if err != nil || f.ID < 0 {
		return f, service.ErrInvalid
	}
	f.Version, err = strconv.ParseInt(c.FormValue("version"), 10, 64)
	if err != nil || f.Version < 0 {
		return f, service.ErrInvalid
	}
	labels, types, options := c.Request().PostForm["field_label"], c.Request().PostForm["field_type"], c.Request().PostForm["field_options"]
	required := c.Request().PostForm["field_required"]
	if len(labels) > 20 || len(labels) != len(types) || len(labels) != len(options) || len(labels) != len(required) {
		return f, service.ErrInvalid
	}
	for i, label := range labels {
		field := store.FormField{Label: label, Type: types[i], Required: required[i] == "true"}
		if strings.TrimSpace(options[i]) != "" {
			field.Options = strings.Split(strings.ReplaceAll(options[i], "\r\n", "\n"), "\n")
		}
		f.Fields = append(f.Fields, field)
	}
	return f, nil
}
func (a *App) formRoutes(g *echo.Group) {
	a.entryRoutes(g)
	g.GET("/forms", a.formsPage)
	g.GET("/forms/new", func(c *echo.Context) error {
		return a.formEditor(c, 200, store.Form{Enabled: true, Button: i18n.T(c.Request().Context(), "Отправить"), Success: i18n.T(c.Request().Context(), "Спасибо! Ваша заявка отправлена."), Fields: []store.FormField{{Label: i18n.T(c.Request().Context(), "Имя"), Type: "text", Required: true}, {Label: "Email", Type: "email", Required: true}, {Label: i18n.T(c.Request().Context(), "Сообщение"), Type: "textarea", Required: true}}}, "")
	})
	g.GET("/forms/:form/edit", func(c *echo.Context) error {
		id, err := formID(c)
		if err != nil {
			return err
		}
		f, err := a.Service.Store.Form(id)
		if err != nil {
			return err
		}
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Форма сохранена."
		}
		return a.formEditor(c, 200, f, message)
	})
	g.POST("/forms/save", func(c *echo.Context) error {
		f, err := postedForm(c)
		if err != nil {
			return err
		}
		id, err := a.Service.SaveForm(c.Request().Context(), f)
		if errors.Is(err, service.ErrInvalid) {
			return a.formEditor(c, 400, f, "Проверьте название, email получателя и поля. Нужно от 1 до 20 полей; для списка укажите непустые варианты без повторов.")
		}
		if errors.Is(err, store.ErrConflict) {
			return a.formEditor(c, 409, f, "Форма изменена в другой вкладке. Откройте актуальную версию перед сохранением.")
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, formPath(id)+"/edit?saved=1")
	}, a.csrf)
	g.GET("/forms/:form/entries", func(c *echo.Context) error {
		id, err := formID(c)
		if err != nil {
			return err
		}
		f, err := a.Service.Store.Form(id)
		if err != nil {
			return err
		}
		page := 1
		if raw := c.QueryParam("page"); raw != "" {
			page, err = strconv.Atoi(raw)
			if err != nil || page < 1 || page > 1000000 {
				return service.ErrInvalid
			}
		}
		entries, total, err := a.Service.Store.FormEntries(id, page)
		if err != nil {
			return err
		}
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, FormEntries(site.Title, a.csrfToken(c), f, entries, page, total, c.QueryParam("mail")))
	})
	g.POST("/forms/:form/entries/:entry/retry", func(c *echo.Context) error {
		id, err := formID(c)
		if err != nil {
			return err
		}
		entry, err := strconv.ParseInt(c.Param("entry"), 10, 64)
		if err != nil || entry < 1 {
			return service.ErrInvalid
		}
		f, err := a.Service.Store.Form(id)
		if err != nil {
			return err
		}
		if err = a.Service.Store.RetryFormEntry(c.Request().Context(), f.ID, entry, time.Now().Unix()); err != nil {
			return err
		}
		return c.Redirect(303, formPath(id)+"/entries?mail=queued")
	}, a.csrf)
	a.Echo.GET("/forms/:form", func(c *echo.Context) error { return a.publicFormPage(c, 200, "") })
	a.Echo.POST("/forms/:form", a.submitForm, a.csrf)
}
func (a *App) publicFormPage(c *echo.Context, status int, message string) error {
	id, err := formID(c)
	if err != nil {
		return err
	}
	f, err := a.Service.Store.Form(id)
	if err != nil {
		return err
	}
	if !f.Enabled {
		return echo.NewHTTPError(404, "Not found")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
	if c.QueryParam("sent") == "1" && message == "" {
		message = f.Success
	}
	var out bytes.Buffer
	if err = PublicFormPage(f, a.csrfToken(c), message, status >= 400, c.Request().PostForm).Render(c.Request().Context(), &out); err != nil {
		return err
	}
	return c.HTML(status, out.String())
}
func (a *App) submitForm(c *echo.Context) error {
	if err := c.Request().ParseForm(); err != nil {
		return service.ErrInvalid
	}
	id, err := formID(c)
	if err != nil {
		return err
	}
	f, err := a.Service.Store.Form(id)
	if err != nil {
		return err
	}
	if !f.Enabled {
		return echo.NewHTTPError(404, "Not found")
	}
	if c.FormValue("website") != "" {
		return c.Redirect(303, "/forms/"+strconv.FormatInt(id, 10)+"?sent=1")
	}
	if !a.Service.AllowFormSubmission(c.RealIP()) {
		return service.ErrRateLimit
	}
	version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
	if err != nil || version != f.Version {
		return a.publicFormPage(c, 409, "Форма обновилась. Проверьте поля и отправьте её ещё раз.")
	}
	values, err := service.FormValues(f, c.Request().PostForm)
	if err != nil {
		return a.publicFormPage(c, 400, "Проверьте обязательные поля, email и выбранные варианты.")
	}
	nonce := c.FormValue("submission_key")
	if len(nonce) != 64 {
		return service.ErrInvalid
	}
	_, err = a.Service.Store.SaveFormEntry(c.Request().Context(), f, values, service.Hash(strconv.FormatInt(f.ID, 10)+":"+a.csrfToken(c)+":"+nonce))
	if err != nil {
		return err
	}
	return c.Redirect(303, "/forms/"+strconv.FormatInt(id, 10)+"?sent=1")
}

var formShortcode = regexp.MustCompile(`<p(?: [^>]*)?>\[form id=([1-9][0-9]{0,18})\]</p>`)

func (a *App) renderForms(c *echo.Context, body string) (string, error) {
	var csrf string
	return replaceContentBlocks(body, formShortcode, func(code string) (string, error) {
		id, err := strconv.ParseInt(formShortcode.FindStringSubmatch(code)[1], 10, 64)
		if err != nil {
			return code, nil
		}
		f, err := a.Service.Store.Form(id)
		if errors.Is(err, sql.ErrNoRows) || err == nil && !f.Enabled {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		if csrf == "" {
			csrf = a.csrfToken(c)
		}
		out := themeOutput{limit: 2 << 20}
		err = PublicForm(f, csrf, nil).Render(c.Request().Context(), &out)
		return out.String(), err
	})
}
