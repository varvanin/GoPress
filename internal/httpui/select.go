package httpui

import (
	"context"
	"gopress/internal/i18n"
	"gopress/internal/store"
	"gopress/internal/ui/components/select"
	"strconv"
)

func selectedTermValue(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
func termSelectItems(ctx context.Context, terms []store.Term) []selectcomp.ItemData {
	items := []selectcomp.ItemData{{Value: "", Label: i18n.T(ctx, "Все категории и теги")}}
	for _, term := range terms {
		items = append(items, selectcomp.ItemData{Value: strconv.FormatInt(term.ID, 10), Label: term.Name})
	}
	return items
}
func menuPageItems(ctx context.Context, pages []store.Page) []selectcomp.ItemData {
	items := []selectcomp.ItemData{{Value: "/", Label: i18n.T(ctx, "Главная")}}
	for _, page := range pages {
		items = append(items, selectcomp.ItemData{Value: publicContentURL(page.Slug, page.URL), Label: page.Title})
	}
	return items
}
