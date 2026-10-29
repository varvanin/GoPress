package httpui

import (
	"bytes"
	"database/sql"
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
	"regexp"
	"strconv"
	"strings"
)

func faqPath(id int64) string { return "/admin/modules/faq/" + strconv.FormatInt(id, 10) + "/edit" }
func faqCode(id int64) string { return "[faq id=" + strconv.FormatInt(id, 10) + "]" }
func (a *App) faqEditor(c *echo.Context, status int, f store.FAQGroup, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, FAQEditor(site.Title, a.csrfToken(c), f, message))
}
func (a *App) moduleRoutes(g *echo.Group) {
	a.packageModuleRoutes(g)
	g.GET("/modules", func(c *echo.Context) error {
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		modules, err := a.moduleCatalog()
		if err != nil {
			return err
		}
		query, state := strings.TrimSpace(c.QueryParam("q")), c.QueryParam("state")
		if len(query) > 200 || state != "" && state != "enabled" && state != "disabled" {
			return service.ErrInvalid
		}
		total := len(modules)
		filtered := make([]ModuleInfo, 0, total)
		for _, module := range modules {
			if state == "enabled" && !module.Enabled || state == "disabled" && module.Enabled {
				continue
			}
			description := module.Description
			if module.Builtin {
				description = i18n.T(c.Request().Context(), description)
			}
			text := strings.ToLower(strings.Join([]string{module.ID, module.Name, module.Author, description}, " "))
			if strings.Contains(text, strings.ToLower(query)) {
				filtered = append(filtered, module)
			}
		}
		return htmlComponent(c, 200, Modules(site.Title, a.csrfToken(c), filtered, c.QueryParam("installed") == "1", query, state, total))
	})
	g.POST("/modules/faq/toggle", func(c *echo.Context) error {
		location, err := moduleCatalogLocation(c)
		if err != nil {
			return err
		}
		value := c.FormValue("enabled")
		if value != "true" && value != "false" {
			return service.ErrInvalid
		}
		if err := a.Service.Store.SetFAQEnabled(c.Request().Context(), value == "true"); err != nil {
			return err
		}
		return c.Redirect(303, location)
	}, a.csrf)
	g.GET("/modules/faq", func(c *echo.Context) error {
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		groups, err := a.Service.Store.FAQGroups()
		if err != nil {
			return err
		}
		enabled, err := a.Service.Store.FAQEnabled()
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, FAQList(site.Title, a.csrfToken(c), groups, enabled))
	})
	g.GET("/modules/faq/new", func(c *echo.Context) error {
		return a.faqEditor(c, 200, store.FAQGroup{Items: []store.FAQItem{{}}}, "")
	})
	g.GET("/modules/faq/:faq/edit", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.Param("faq"), 10, 64)
		if err != nil || id < 1 {
			return service.ErrInvalid
		}
		f, err := a.Service.Store.FAQ(id)
		if err != nil {
			return err
		}
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Группа FAQ сохранена."
		}
		return a.faqEditor(c, 200, f, message)
	})
	g.POST("/modules/faq/save", func(c *echo.Context) error {
		if err := c.Request().ParseForm(); err != nil {
			return service.ErrInvalid
		}
		f := store.FAQGroup{Name: c.FormValue("name")}
		var err error
		f.ID, err = strconv.ParseInt(c.FormValue("id"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		f.Version, err = strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		questions, answers := c.Request().PostForm["question"], c.Request().PostForm["answer"]
		if len(questions) != len(answers) || len(questions) > 50 {
			return service.ErrInvalid
		}
		for i, q := range questions {
			f.Items = append(f.Items, store.FAQItem{Question: q, Answer: answers[i]})
		}
		id, err := a.Service.SaveFAQ(c.Request().Context(), f)
		if errors.Is(err, service.ErrInvalid) {
			return a.faqEditor(c, 400, f, "Укажите название и от 1 до 50 вопросов с ответами.")
		}
		if errors.Is(err, store.ErrConflict) {
			return a.faqEditor(c, 409, f, "Группа изменена в другой вкладке. Откройте актуальную версию.")
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, faqPath(id)+"?saved=1")
	}, a.csrf)
	g.POST("/modules/faq/delete", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.FormValue("id"), 10, 64)
		if err != nil || id < 1 {
			return service.ErrInvalid
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil || version < 1 {
			return service.ErrInvalid
		}
		if err = a.Service.Store.DeleteFAQ(c.Request().Context(), id, version); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/modules/faq")
	}, a.csrf)
}

var faqShortcode = regexp.MustCompile(`<p(?: [^>]*)?>\[faq id=([1-9][0-9]{0,18})\]</p>`)

func (a *App) renderFAQ(c *echo.Context, body string) (string, error) {
	if !faqShortcode.MatchString(body) {
		return body, nil
	}
	enabled, err := a.Service.Store.FAQEnabled()
	if err != nil {
		return "", err
	}
	var renderErr error
	body = faqShortcode.ReplaceAllStringFunc(body, func(code string) string {
		if !enabled || renderErr != nil {
			return ""
		}
		id, err := strconv.ParseInt(faqShortcode.FindStringSubmatch(code)[1], 10, 64)
		if err != nil {
			return code
		}
		f, err := a.Service.Store.FAQ(id)
		if errors.Is(err, sql.ErrNoRows) {
			return ""
		}
		if err != nil {
			renderErr = err
			return ""
		}
		var out bytes.Buffer
		renderErr = PublicFAQ(f).Render(c.Request().Context(), &out)
		return out.String()
	})
	return body, renderErr
}
