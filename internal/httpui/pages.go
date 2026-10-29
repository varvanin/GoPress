package httpui

import (
	"context"
	"encoding/json"
	"gopress/internal/i18n"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopress/internal/service"
	"gopress/internal/store"
	selectcomp "gopress/internal/ui/components/select"
)

func pageLeaf(slug string) string {
	if slug == "" {
		return ""
	}
	return path.Base(slug)
}
func (a *App) pageOptions(ctx context.Context, p store.Page) ([]selectcomp.ItemData, []selectcomp.ItemData, error) {
	pages, err := a.Service.Store.HomePages()
	if err != nil {
		return nil, nil, err
	}
	parents := []selectcomp.ItemData{{Value: "0", Label: i18n.T(ctx, "Без родителя")}}
	for _, page := range pages {
		if page.ID != p.ID {
			parents = append(parents, selectcomp.ItemData{Value: strconv.FormatInt(page.ID, 10), Label: page.Title})
		}
	}
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return nil, nil, err
	}
	source, prefix, _, err := a.themeSource(appearance.Theme)
	if err != nil {
		return nil, nil, err
	}
	dir := strings.TrimSuffix(prefix, "/")
	if dir == "" {
		dir = "."
	}
	entries, err := fs.ReadDir(source, dir)
	if err != nil {
		return nil, nil, err
	}
	templates := []selectcomp.ItemData{{Value: "page.html", Label: i18n.T(ctx, "Стандартный")}}
	found := p.Template == "" || p.Template == "page.html"
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "page.html" && service.ValidPageTemplate(entry.Name()) {
			templates = append(templates, selectcomp.ItemData{Value: entry.Name(), Label: entry.Name()})
			found = found || entry.Name() == p.Template
		}
	}
	if !found {
		templates = append(templates, selectcomp.ItemData{Value: p.Template, Label: p.Template + " " + i18n.T(ctx, "(недоступен)")})
	}
	return parents, templates, nil
}
func (a *App) checkPageTemplate(p store.Page) error {
	if p.Template == "" || p.Template == "page.html" {
		return nil
	}
	if !service.ValidPageTemplate(p.Template) {
		return service.ErrInvalid
	}
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	source, prefix, _, err := a.themeSource(appearance.Theme)
	if err != nil {
		return err
	}
	if _, err = fs.Stat(source, prefix+p.Template); err != nil {
		return service.ErrInvalid
	}
	return nil
}

func translationJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
func sourceContentLanguage(p store.Page) string {
	if p.SEO.Language != "" {
		return p.SEO.Language
	}
	return "ru"
}
func materialLanguages() []selectcomp.ItemData {
	return []selectcomp.ItemData{{Value: "en", Label: "English"}, {Value: "ru", Label: "Русский"}, {Value: "pl", Label: "Polski"}, {Value: "de", Label: "Deutsch"}, {Value: "uk", Label: "Українська"}, {Value: "it", Label: "Italiano"}, {Value: "es", Label: "Español"}, {Value: "cs", Label: "Čeština"}}
}
func materialLanguageLabel(code string) string {
	for _, item := range materialLanguages() {
		if item.Value == code {
			return item.Label.(string)
		}
	}
	return code
}
func contentLanguageOptions(p store.Page) []selectcomp.ItemData {
	items := []selectcomp.ItemData{{Value: "original", Label: materialLanguageLabel(sourceContentLanguage(p))}}
	for _, language := range sortedTranslationLanguages(p.Translations) {
		items = append(items, selectcomp.ItemData{Value: language, Label: materialLanguageLabel(language)})
	}
	return items
}
func availableContentLanguages(p store.Page) []selectcomp.ItemData {
	items := []selectcomp.ItemData{}
	for _, item := range materialLanguages() {
		if _, exists := p.Translations[item.Value]; !exists && item.Value != sourceContentLanguage(p) {
			items = append(items, item)
		}
	}
	return items
}
func sortedTranslationLanguages(translations map[string]store.Translation) []string {
	languages := make([]string, 0, len(translations))
	for language := range translations {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}
