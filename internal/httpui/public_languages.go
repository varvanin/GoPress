package httpui

import (
	"context"
	"net/url"
	"strings"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

type PublicLanguage struct {
	Code, Label, URL string
	Current          bool
}

func languageURL(path, language string) string {
	u, err := url.Parse(path)
	if err != nil {
		return path
	}
	q := u.Query()
	if language == "" || language == "original" {
		q.Del("lang")
	} else {
		q.Set("lang", language)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func publicLanguageLinks(p store.Page, path, active string) []PublicLanguage {
	original := sourceContentLanguage(p)
	links := []PublicLanguage{{Code: original, Label: materialLanguageLabel(original), URL: languageURL(path, ""), Current: active == "" || active == "original" || active == original}}
	for _, code := range sortedTranslationLanguages(p.Translations) {
		if code == original {
			continue
		}
		links = append(links, PublicLanguage{Code: code, Label: materialLanguageLabel(code), URL: languageURL(path, code), Current: active == code})
	}
	return links
}
func publicText(language, text string) string {
	return i18n.T(i18n.WithLanguage(context.Background(), strings.Split(language, "-")[0]), text)
}
func publicContentURL(slug, url string) string {
	if url != "" {
		return url
	}
	return "/" + slug
}
func (a *App) publicMenu(position, language string) (store.Menu, error) {
	if language != "" {
		m, err := a.Service.Store.Menu(position, language)
		if err != nil || m.Version > 0 {
			return m, err
		}
	}
	return a.Service.Store.Menu(position)
}
