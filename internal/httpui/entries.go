package httpui

import (
	"encoding/csv"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
)

func csvText(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	return value
}
func (a *App) entryRoutes(g *echo.Group) {
	g.POST("/forms/:form/entries/delete", func(c *echo.Context) error {
		id, err := formID(c)
		if err != nil {
			return err
		}
		if err = c.Request().ParseForm(); err != nil {
			return service.ErrInvalid
		}
		values := c.Request().PostForm["selection"]
		if len(values) < 1 || len(values) > 25 {
			return service.ErrInvalid
		}
		ids := []int64{}
		seen := map[int64]bool{}
		for _, value := range values {
			entry, err := strconv.ParseInt(value, 10, 64)
			if err != nil || entry < 1 || seen[entry] {
				return service.ErrInvalid
			}
			seen[entry] = true
			ids = append(ids, entry)
		}
		if err = a.Service.Store.DeleteFormEntries(c.Request().Context(), id, ids, time.Now().Unix()); err != nil {
			return err
		}
		return c.Redirect(303, formPath(id)+"/entries")
	}, a.csrf)
	g.POST("/forms/retention", func(c *echo.Context) error {
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
		settings.FormRetentionDays = days
		if err = a.Service.SaveSettings(c.Request().Context(), settings); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/forms")
	}, a.csrf)
	g.GET("/forms/:form/entries/export", func(c *echo.Context) error {
		id, err := formID(c)
		if err != nil {
			return err
		}
		if _, err = a.Service.Store.Form(id); err != nil {
			return err
		}
		upper, err := a.Service.Store.LastFormEntry(id)
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Type", "text/csv; charset=utf-8")
		c.Response().Header().Set("Content-Disposition", `attachment; filename="form-`+strconv.FormatInt(id, 10)+`.csv"`)
		writer := csv.NewWriter(c.Response())
		if err = writer.Write([]string{"submission_id", "submitted_at_utc", "field", "value"}); err != nil {
			return err
		}
		after := int64(0)
		for {
			entries, err := a.Service.Store.FormEntryBatch(id, after, upper)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				break
			}
			for _, entry := range entries {
				for _, value := range entry.Values {
					if err = writer.Write([]string{strconv.FormatInt(entry.ID, 10), time.Unix(entry.CreatedAt, 0).UTC().Format(time.RFC3339), csvText(value.Label), csvText(value.Value)}); err != nil {
						return err
					}
				}
				after = entry.ID
			}
			writer.Flush()
			if err = writer.Error(); err != nil {
				return err
			}
		}
		writer.Flush()
		if err = writer.Error(); err != nil {
			return err
		}
		return a.Service.Store.Audit(c.Request().Context(), "forms.export", strconv.FormatInt(id, 10))
	})
}
