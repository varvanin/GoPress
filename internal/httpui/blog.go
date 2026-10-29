package httpui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func postedPage(c *echo.Context) (store.Page, error) {
	if err := c.Request().ParseForm(); err != nil {
		return store.Page{}, service.ErrInvalid
	}
	p := store.Page{Title: c.FormValue("title"), Slug: c.FormValue("slug"), Document: c.FormValue("document"), Cover: c.FormValue("cover"), SEO: store.SEO{Canonical: c.FormValue("seo_canonical"), SocialTitle: c.FormValue("seo_social_title"), SocialDescription: c.FormValue("seo_social_description"), SocialImage: c.FormValue("seo_social_image"), NoFollow: c.FormValue("seo_nofollow") == "1", Title: c.FormValue("seo_title"), Description: c.FormValue("seo_description"), NoIndex: c.FormValue("noindex") == "1"}, Kind: c.FormValue("kind")}
	if raw := c.FormValue("translations"); raw != "" {
		if len(raw) > 2*service.MaxContentSize+(1<<20) || json.Unmarshal([]byte(raw), &p.Translations) != nil {
			return p, service.ErrInvalid
		}
	}
	var err error
	p.ID, err = strconv.ParseInt(c.FormValue("id"), 10, 64)
	if err != nil || p.ID < 0 {
		return p, service.ErrInvalid
	}
	p.Version, err = strconv.ParseInt(c.FormValue("version"), 10, 64)
	if err != nil || p.Version < 0 {
		return p, service.ErrInvalid
	}
	if raw := c.FormValue("parent_id"); raw != "" {
		p.ParentID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return p, service.ErrInvalid
		}
	}
	if raw := c.FormValue("order"); raw != "" {
		p.Order, err = strconv.Atoi(raw)
		if err != nil {
			return p, service.ErrInvalid
		}
	}
	p.Template = c.FormValue("template")
	for _, raw := range c.Request().PostForm["term_ids"] {
		id, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || id < 1 || len(p.TermIDs) >= 100 {
			return p, service.ErrInvalid
		}
		p.TermIDs = append(p.TermIDs, id)
	}
	if language := c.FormValue("content_language"); language != "" && language != "original" {
		var original store.Translation
		if json.Unmarshal([]byte(c.FormValue("original_translation")), &original) != nil {
			return p, service.ErrInvalid
		}
		if p.Translations == nil {
			p.Translations = map[string]store.Translation{}
		}
		p.Translations[language] = store.Translation{Title: p.Title, Document: p.Document, SEO: p.SEO}
		p.Title, p.Document, p.SEO = original.Title, original.Document, original.SEO
	}
	if source := c.FormValue("source_language"); source != "" {
		p.SEO.Language = source
	}
	return p, nil
}
func selectedTerm(ids []int64, id int64) bool {
	for _, value := range ids {
		if value == id {
			return true
		}
	}
	return false
}
func scheduleValue(at int64, zone string) string {
	if at == 0 {
		return ""
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	return time.Unix(at, 0).In(loc).Format("2006-01-02T15:04")
}
func (a *App) termsPage(c *echo.Context, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	terms, err := a.Service.Store.Terms(false)
	if err != nil {
		return err
	}
	return htmlComponent(c, status, Terms(site.Title, a.csrfToken(c), terms, message, c.FormValue("name")))
}
func (a *App) blogRoutes(g *echo.Group) {
	g.GET("/posts/terms", func(c *echo.Context) error { return a.termsPage(c, 200, "") })
	g.POST("/posts/terms", func(c *echo.Context) error {
		id, err := strconv.ParseInt(c.FormValue("id"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		action := c.FormValue("action")
		if action != "save" && action != "delete" {
			return service.ErrInvalid
		}
		err = a.Service.SaveTerm(c.Request().Context(), store.Term{ID: id, Version: version, Name: c.FormValue("name"), Kind: c.FormValue("kind")}, action == "delete")
		if err != nil {
			if errors.Is(err, service.ErrInvalid) {
				return a.termsPage(c, 400, "Название должно содержать от 1 до 80 символов.")
			}
			if errors.Is(err, store.ErrConflict) || strings.Contains(err.Error(), "constraint failed") {
				return a.termsPage(c, 409, "Название уже занято или раздел изменён в другом окне. Проверьте актуальный список.")
			}
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/posts/terms")
	}, a.csrf)
	g.POST("/pages/:id/schedule", func(c *echo.Context) error {
		id, err := pageID(c)
		if err != nil {
			return err
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil || version < 1 {
			return service.ErrInvalid
		}
		if err = a.Service.Schedule(c.Request().Context(), id, version, c.FormValue("publish_at")); err != nil {
			if errors.Is(err, service.ErrInvalid) {
				p, e := a.Service.Store.Page(id)
				if e != nil {
					return e
				}
				return a.editPage(c, p, 400, "Выберите однозначное будущее время в часовом поясе сайта. В часы перевода часов выберите время вне повторяющегося или пропущенного интервала.")
			}
			return err
		}
		return c.Redirect(303, "/admin/pages/"+strconv.FormatInt(id, 10)+"/edit?done=scheduled")
	}, a.csrf)
}

func termKind(kind string) string {
	if kind == "category" {
		return "Категории"
	}
	return "Теги"
}
