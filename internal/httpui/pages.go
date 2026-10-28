package httpui

import (
	"context"
	"gopress/internal/i18n"
	"io/fs"
	"path"
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
