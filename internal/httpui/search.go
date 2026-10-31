package httpui

import (
	"bytes"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/i18n"
	"gopress/internal/service"
	"gopress/internal/store"
)

type PublicSearchData struct {
	Language    string
	Query       string
	Page, Total int
	Results     []store.SearchResult
}

func (a *App) search(c *echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("q"))
	page, err := pageNumber(c)
	if err != nil {
		return err
	}
	if len([]rune(query)) > 200 || len(strings.Fields(query)) > 10 {
		return service.ErrInvalid
	}
	language := c.QueryParam("lang")
	if language == "original" {
		language = ""
	}
	if language != "" && !service.ValidContentLanguage(language) {
		return service.ErrInvalid
	}
	results, total, err := a.Service.Store.SearchPublic(query, page, language)
	if err != nil {
		return err
	}
	c.Set("public-search", &PublicSearchData{Language: language, Query: query, Page: page, Total: total, Results: results})
	return a.render(c, store.Page{Title: "Поиск", Slug: "search", Document: emptyDocument, SEO: store.SEO{NoIndex: true}}, false)
}
func searchBody(c *echo.Context, data *PublicSearchData) (string, error) {
	var out bytes.Buffer
	err := SearchResults(*data).Render(i18n.WithLanguage(c.Request().Context(), data.Language), &out)
	return out.String(), err
}
