package httpui

import (
	"context"
	"gopress/internal/i18n"
	"gopress/internal/store"
	selectcomp "gopress/internal/ui/components/select"
)

func blockLabel(kind string) string {
	switch kind {
	case "faq":
		return "FAQ"
	case "form":
		return "Форма"
	case "gallery":
		return "Галерея"
	case "testimonials":
		return "Отзывы"
	}
	return "Модуль"
}
func blockCode(b store.ContentBlock) string {
	if b.Kind == "module" {
		return `[module id="` + b.ID + `"]`
	}
	return "[" + b.Kind + " id=" + b.ID + "]"
}
func blockChoices(ctx context.Context, blocks []store.ContentBlock) []selectcomp.ItemData {
	items := []selectcomp.ItemData{}
	for _, b := range blocks {
		items = append(items, selectcomp.ItemData{Value: blockCode(b), Label: i18n.T(ctx, blockLabel(b.Kind)) + " · " + b.Name})
	}
	return items
}
