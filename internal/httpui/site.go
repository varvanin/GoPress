package httpui

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func mediaPageNumber(c *echo.Context) (int, error) {
	page := 1
	if raw := c.QueryParam("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000000 {
			return 0, service.ErrInvalid
		}
		page = n
	}
	if len(c.QueryParam("q")) > 200 {
		return 0, service.ErrInvalid
	}
	return page, nil
}
func mediaURL(query string, page int) string {
	return "/admin/media?" + url.Values{"q": {query}, "page": {strconv.Itoa(page)}}.Encode()
}
func (a *App) mediaLibrary(c *echo.Context, status int, message string, posted *store.MediaFile) error {
	if c.Request().Method == "GET" && message == "" {
		if c.QueryParam("uploaded") == "1" {
			message = "Изображение загружено."
		} else if c.QueryParam("saved") == "1" {
			message = "Сведения сохранены."
		}
	}
	page, err := mediaPageNumber(c)
	if err != nil {
		return err
	}
	query := c.QueryParam("q")
	if posted != nil {
		query = c.FormValue("return_q")
		if n, e := strconv.Atoi(c.FormValue("return_page")); e == nil && n >= 1 && n <= 1000000 {
			page = n
		}
	}
	files, total, err := a.Service.Store.ListMedia(query, page, mediaOwner(c.Request().Context()))
	if err != nil {
		return err
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	var selected *store.MediaFile
	used := false
	name := c.QueryParam("file")
	if posted != nil {
		name = posted.Name
	}
	if name == "" && len(files) > 0 {
		name = files[0].Name
	}
	if name != "" {
		file, e := a.Service.Store.MediaFile(name)
		if e != nil {
			return e
		}
		if posted != nil {
			file.Title, file.Alt, file.Caption, file.Revision = posted.Title, posted.Alt, posted.Caption, posted.Revision
		}
		selected = &file
		used, e = a.Service.Store.MediaUsed(name)
		if e != nil {
			return e
		}
	}
	return htmlComponent(c, status, MediaLibrary(site.Title, site.BaseURL, a.csrfToken(c), files, total, page, query, message, selected, used, status >= 400))
}
func (a *App) menusPage(c *echo.Context, status int, message string, posted *store.Menu) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	language := c.QueryParam("lang")
	position := c.QueryParam("position")
	if posted != nil {
		position = posted.Position
		language = posted.Language
	}
	if position == "" {
		position = "header"
	}
	if position != "header" && position != "footer" {
		return service.ErrInvalid
	}
	if language != "" && !service.ValidContentLanguage(language) {
		return service.ErrInvalid
	}
	menu, err := a.Service.Store.Menu(position, language)
	if err != nil {
		return err
	}
	if posted != nil {
		menu = *posted
	}
	pages, err := a.Service.Store.PublishedContent("page", language)
	if err != nil {
		return err
	}
	return htmlComponent(c, status, Menus(site.Title, a.csrfToken(c), menu, pages, message, status >= 400))
}
func (a *App) siteRoutes(g *echo.Group) {
	g.POST("/media/upload", func(c *echo.Context) error {
		c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, service.MaxUpload+(1<<20))
		if err := c.Request().ParseMultipartForm(1 << 20); err != nil {
			return service.ErrInvalid
		}
		defer c.Request().MultipartForm.RemoveAll()
		f, header, err := c.Request().FormFile("file")
		if err != nil {
			return service.ErrInvalid
		}
		defer f.Close()
		name, err := a.Service.Upload(f, c.Request().Context())
		if err != nil {
			return a.mediaLibrary(c, 400, "Не удалось загрузить изображение. Нужен PNG или JPEG до 8 MiB и 16 миллионов пикселей.", nil)
		}
		title := header.Filename
		if len(title) > 200 {
			title = ""
		}
		details, err := a.Service.Store.MediaFile(name)
		if err != nil {
			return err
		}
		details.Title = title
		if err = a.Service.SaveMediaDetails(details); err != nil {
			return err
		}
		return c.Redirect(303, mediaFileURL("", 1, name)+"&uploaded=1")
	}, a.csrf)
	g.POST("/media/details", func(c *echo.Context) error {
		returnPage, _ := strconv.Atoi(c.FormValue("return_page"))
		if returnPage < 1 || returnPage > 1000000 {
			returnPage = 1
		}
		returnQuery := c.FormValue("return_q")
		if len(returnQuery) > 200 {
			return service.ErrInvalid
		}

		f := store.MediaFile{Name: c.FormValue("name"), Title: c.FormValue("title"), Alt: c.FormValue("alt"), Caption: c.FormValue("caption"), Revision: c.FormValue("revision")}
		if err := a.Service.SaveMediaDetails(f); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return a.mediaLibrary(c, 409, "Сведения изменены в другой вкладке. Скопируйте свои правки и откройте актуальную версию.", &f)
			}
			if errors.Is(err, service.ErrInvalid) {
				return a.mediaLibrary(c, 400, "Проверьте длину названия, alt и подписи.", &f)
			}
			return err
		}
		return c.Redirect(303, mediaFileURL(returnQuery, returnPage, f.Name)+"&saved=1")
	}, a.csrf)
	g.POST("/media/delete", func(c *echo.Context) error {
		err := a.Service.DeleteMedia(c.Request().Context(), c.FormValue("name"))
		if errors.Is(err, store.ErrMediaUsed) {
			return a.mediaLibrary(c, 409, "Изображение используется в материалах, настройках сайта или модулей. Его нельзя удалить.", nil)
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, "/admin/media")
	}, a.csrf)
	g.GET("/menus", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Меню сохранено."
		}
		return a.menusPage(c, 200, message, nil)
	})
	g.POST("/menus", func(c *echo.Context) error {
		if err := c.Request().ParseForm(); err != nil {
			return service.ErrInvalid
		}
		version, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
		if err != nil {
			return service.ErrInvalid
		}
		m := store.Menu{Language: c.FormValue("language"), Position: c.FormValue("position"), Version: version, Items: []store.MenuItem{}}
		labels, urls, depths := c.Request().PostForm["label"], c.Request().PostForm["url"], c.Request().PostForm["depth"]
		if len(labels) != len(urls) || len(labels) != len(depths) || len(labels) > 51 {
			return service.ErrInvalid
		}
		for i, label := range labels {
			if strings.TrimSpace(label) == "" && strings.TrimSpace(urls[i]) == "" {
				continue
			}
			depth, e := strconv.Atoi(depths[i])
			if e != nil {
				return service.ErrInvalid
			}
			m.Items = append(m.Items, store.MenuItem{Label: label, URL: urls[i], Depth: depth})
		}
		err = a.Service.SaveMenu(c.Request().Context(), m)
		if errors.Is(err, store.ErrConflict) {
			return a.menusPage(c, 409, "Меню уже изменено. Обновите страницу перед повторным сохранением.", &m)
		}
		if errors.Is(err, service.ErrInvalid) {
			return a.menusPage(c, 400, "Проверьте названия и адреса. Подпункт должен идти после основного пункта; допускается до трёх уровней.", &m)
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, "/admin/menus?position="+m.Position+"&lang="+url.QueryEscape(m.Language)+"&saved=1")
	}, a.csrf)
}
func mediaTitle(f store.MediaFile) string {
	if f.Title != "" {
		return f.Title
	}
	return f.Name[:min(12, len(f.Name))] + "…"
}
func mediaSize(size int64) string { return fmt.Sprintf("%.0f KiB", float64(size)/1024) }

type MenuLink struct {
	Label, URL string
	Children   []MenuLink
}

func menuLinks(items []store.MenuItem) []MenuLink {
	var build func(*int, int) []MenuLink
	build = func(index *int, depth int) []MenuLink {
		links := []MenuLink{}
		for *index < len(items) && items[*index].Depth == depth {
			item := items[*index]
			*index = *index + 1
			link := MenuLink{Label: item.Label, URL: item.URL}
			if *index < len(items) && items[*index].Depth > depth {
				link.Children = build(index, depth+1)
			}
			links = append(links, link)
		}
		return links
	}
	index := 0
	return build(&index, 0)
}

func (a *App) mediaSelection(query string, page int, selected string, owners ...int64) ([]store.MediaFile, int, error) {
	files, total, err := a.Service.Store.ListMedia(query, page, owners...)
	if err != nil {
		return nil, 0, err
	}
	if selected != "" {
		found := false
		for _, file := range files {
			if file.Name == selected {
				found = true
				break
			}
		}
		if !found {
			file, e := a.Service.Store.MediaFile(selected)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return nil, 0, e
			}
			if e == nil {
				if len(owners) > 0 && owners[0] > 0 {
					owner, err := a.Service.Store.MediaOwner(file.Name)
					if err != nil {
						return nil, 0, err
					}
					if owner != owners[0] {
						return files, total, nil
					}
				}
				files = append([]store.MediaFile{file}, files...)
			}
		}
	}
	return files, total, nil
}
func pickerURL(query string, page int) string {
	return "/admin/media/choices?" + url.Values{"q": {query}, "page": {strconv.Itoa(page)}}.Encode()
}

func mediaFormat(name string) string {
	if strings.HasSuffix(name, ".png") {
		return "PNG"
	}
	return "JPEG"
}

func mediaFileURL(query string, page int, name string) string {
	values := url.Values{"page": {strconv.Itoa(page)}}
	if query != "" {
		values.Set("q", query)
	}
	if name != "" {
		values.Set("file", name)
	}
	return "/admin/media?" + values.Encode()
}
func mediaSelected(selected *store.MediaFile, name string) bool {
	return selected != nil && selected.Name == name
}
