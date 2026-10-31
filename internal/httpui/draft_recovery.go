package httpui

import (
	"context"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

type recoveredDraftKey struct{}

func recoveredDraft(ctx context.Context) bool {
	value, _ := ctx.Value(recoveredDraftKey{}).(bool)
	return value
}
func editorUserID(ctx context.Context) string {
	user, _ := store.Actor(ctx)
	return strconv.FormatInt(user.ID, 10)
}

// Recovery only renders an editor; the normal save still validates and checks its version.
func (a *App) recoverDraft(c *echo.Context) error {
	id := c.FormValue("id")
	version := c.FormValue("version")
	raw := c.FormValue("snapshot")
	if len(raw) == 0 || len(raw) > service.MaxContentSize {
		return service.ErrInvalid
	}
	fields, err := url.ParseQuery(raw)
	if err != nil || fields.Get("id") != id || fields.Get("version") != version {
		return service.ErrInvalid
	}
	c.Request().PostForm = fields
	c.Request().Form = fields
	p, err := postedPage(c)
	if err != nil {
		return err
	}
	if p.ID > 0 {
		current, e := a.Service.Store.Page(p.ID)
		if e != nil {
			return e
		}
		if !service.CanEdit(c.Request().Context(), current) {
			return service.ErrForbidden
		}
		if current.Version != p.Version || current.TrashedAt > 0 {
			return store.ErrConflict
		}
		p.Kind = current.Kind
		p.OwnerID = current.OwnerID
		p.Published = current.Published
		p.PublishedSlug = current.PublishedSlug
		p.CreatedAt = current.CreatedAt
		p.UpdatedAt = current.UpdatedAt
		p.PublishedAt = current.PublishedAt
		p.ScheduledAt = current.ScheduledAt
		p.ScheduledVersion = current.ScheduledVersion
	} else if user, ok := store.Actor(c.Request().Context()); ok && user.Role == "author" && p.Kind != "post" {
		return service.ErrForbidden
	}
	if (p.Kind != "page" && p.Kind != "post") || !service.ValidContentLanguage(sourceContentLanguage(p)) {
		return service.ErrInvalid
	}
	total := len(p.Document)
	if _, err = service.Render(p.Document); err != nil {
		return err
	}
	if len(p.Translations) > 20 {
		return service.ErrInvalid
	}
	for language, tr := range p.Translations {
		total += len(tr.Document)
		if total > service.MaxContentSize || !service.ValidContentLanguage(language) {
			return service.ErrInvalid
		}
		if _, err = service.Render(tr.Document); err != nil {
			return err
		}
	}
	q := c.Request().URL.Query()
	q.Set("lang", fields.Get("content_language"))
	c.Request().URL.RawQuery = q.Encode()
	c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), recoveredDraftKey{}, true)))
	target := "/admin/pages/" + strconv.FormatInt(p.ID, 10) + "/edit"
	if p.ID == 0 {
		target = contentPath(p.Kind) + "/new"
	}
	c.Response().Header().Set("HX-Push-Url", languageURL(target, fields.Get("content_language")))
	return a.editPage(c, p, 200, "Локальный черновик восстановлен в редактор. Проверьте и сохраните его.")
}
