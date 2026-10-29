package httpui

import (
	"database/sql"
	"errors"
	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func collectionTitle(kind string) string {
	if kind == "gallery" {
		return "Галерея"
	}
	return "Отзывы"
}
func collectionPath(kind string, id int64) string {
	return "/admin/modules/" + kind + "/" + strconv.FormatInt(id, 10) + "/edit"
}
func collectionCode(c store.Collection) string {
	return "[" + c.Kind + " id=" + strconv.FormatInt(c.ID, 10) + "]"
}
func (a *App) collectionEditor(c *echo.Context, status int, item store.Collection, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, CollectionEditor(site.Title, a.csrfToken(c), item, message))
}
func (a *App) collectionRoutes(g *echo.Group) {
	g.GET("/modules/media/choices", func(c *echo.Context) error {
		page, _ := strconv.Atoi(c.QueryParam("page"))
		if page < 1 {
			page = 1
		}
		q := c.QueryParam("q")
		if len(q) > 200 {
			return service.ErrInvalid
		}
		files, total, err := a.Service.Store.ListMedia(q, page)
		if err != nil {
			return err
		}
		return htmlComponent(c, 200, CollectionMediaPicker(files, total, page, q))
	})
	for _, kind := range []string{"gallery", "testimonials"} {
		root := "/modules/" + kind
		g.GET(root, func(c *echo.Context) error {
			site, err := a.Service.Store.Site()
			if err != nil {
				return err
			}
			items, err := a.Service.Store.Collections(kind)
			if err != nil {
				return err
			}
			enabled, err := a.Service.Store.ModuleEnabled(kind)
			if err != nil {
				return err
			}
			return htmlComponent(c, 200, Collections(site.Title, a.csrfToken(c), kind, items, enabled))
		})
		g.GET(root+"/new", func(c *echo.Context) error {
			return a.collectionEditor(c, 200, store.Collection{Kind: kind, Layout: "grid", Items: []store.CollectionItem{{}}}, "")
		})
		g.GET(root+"/:collection/edit", func(c *echo.Context) error {
			id, err := strconv.ParseInt(c.Param("collection"), 10, 64)
			if err != nil || id < 1 {
				return service.ErrInvalid
			}
			item, err := a.Service.Store.Collection(kind, id)
			if err != nil {
				return err
			}
			message := ""
			if c.QueryParam("saved") == "1" {
				message = "Изменения сохранены."
			}
			return a.collectionEditor(c, 200, item, message)
		})
		g.POST(root+"/toggle", func(c *echo.Context) error {
			location, err := moduleCatalogLocation(c)
			if err != nil {
				return err
			}
			value := c.FormValue("enabled")
			if value != "true" && value != "false" {
				return service.ErrInvalid
			}
			if err = a.Service.Store.SetModuleEnabled(c.Request().Context(), kind, value == "true"); err != nil {
				return err
			}
			return c.Redirect(303, location)
		}, a.csrf)
		g.POST(root+"/save", func(c *echo.Context) error {
			if err := c.Request().ParseForm(); err != nil {
				return service.ErrInvalid
			}
			item := store.Collection{Kind: kind, Name: c.FormValue("name"), Layout: c.FormValue("layout")}
			var err error
			item.ID, err = strconv.ParseInt(c.FormValue("id"), 10, 64)
			if err != nil {
				return service.ErrInvalid
			}
			item.Version, err = strconv.ParseInt(c.FormValue("version"), 10, 64)
			if err != nil {
				return service.ErrInvalid
			}
			fields := c.Request().PostForm
			images := fields["image"]
			if len(images) < 1 || len(images) > 50 {
				return service.ErrInvalid
			}
			for _, name := range []string{"caption", "person", "company", "text"} {
				if len(fields[name]) != len(images) {
					return service.ErrInvalid
				}
			}
			for i, image := range images {
				item.Items = append(item.Items, store.CollectionItem{Image: image, Caption: fields["caption"][i], Name: fields["person"][i], Company: fields["company"][i], Text: fields["text"][i]})
			}
			id, err := a.Service.SaveCollection(c.Request().Context(), item)
			if errors.Is(err, service.ErrInvalid) {
				return a.collectionEditor(c, 400, item, "Проверьте поля и изображения. Допустимо от 1 до 50 элементов.")
			}
			if errors.Is(err, store.ErrConflict) {
				return a.collectionEditor(c, 409, item, "Раздел изменён в другой вкладке. Откройте актуальную версию.")
			}
			if err != nil {
				return err
			}
			return c.Redirect(303, collectionPath(kind, id)+"?saved=1")
		}, a.csrf)
		g.POST(root+"/delete", func(c *echo.Context) error {
			id, err := strconv.ParseInt(c.FormValue("id"), 10, 64)
			if err != nil || id < 1 {
				return service.ErrInvalid
			}
			version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
			if err != nil || version < 1 {
				return service.ErrInvalid
			}
			if err = a.Service.Store.DeleteCollection(c.Request().Context(), kind, id, version); err != nil {
				return err
			}
			return c.Redirect(303, "/admin"+root)
		}, a.csrf)
	}
}
func collectionPickerURL(q string, page int) string {
	return "/admin/modules/media/choices?q=" + url.QueryEscape(q) + "&page=" + strconv.Itoa(page)
}

var collectionShortcode = regexp.MustCompile(`<p(?: [^>]*)?>\[(gallery|testimonials) id=([1-9][0-9]{0,18})\]</p>`)

func (a *App) renderCollections(c *echo.Context, body string) (string, error) {
	if !collectionShortcode.MatchString(body) {
		return body, nil
	}
	cache := map[string]string{}
	return replaceContentBlocks(body, collectionShortcode, func(code string) (string, error) {
		if value, ok := cache[code]; ok {
			return value, nil
		}
		parts := collectionShortcode.FindStringSubmatch(code)
		enabled, err := a.Service.Store.ModuleEnabled(parts[1])
		if err != nil {
			return "", err
		}
		value := ""
		if enabled {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return "", nil
			}
			item, err := a.Service.Store.Collection(parts[1], id)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
			if err == nil {
				names := []string{}
				for _, row := range item.Items {
					if row.Image != "" {
						names = append(names, row.Image)
					}
				}
				files, err := a.Service.Store.MediaFiles(names)
				if err != nil {
					return "", err
				}
				for _, name := range names {
					if _, ok := files[name]; !ok {
						return "", sql.ErrNoRows
					}
				}
				out := themeOutput{limit: 2 << 20}
				if err = PublicCollection(item, files).Render(c.Request().Context(), &out); err != nil {
					return "", err
				}
				value = out.String()
			}
		}
		cache[code] = value
		return value, nil
	})
}
func collectionImageAlt(item store.CollectionItem, file store.MediaFile) string {
	if strings.TrimSpace(file.Alt) != "" {
		return file.Alt
	}
	return item.Caption
}
