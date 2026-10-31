package service

import (
	"context"
	"errors"
	"gopress/internal/store"
	"strconv"
	"strings"
)

func (v *Service) Duplicate(ctx context.Context, id, version int64) (store.Page, error) {
	p, err := v.Store.Page(id)
	if err != nil {
		return p, err
	}
	if !CanEdit(ctx, p) {
		return p, ErrForbidden
	}
	if p.TrashedAt > 0 || p.Version != version {
		return p, store.ErrConflict
	}
	base := strings.Split(p.Slug, "/")
	leaf := base[len(base)-1]
	if len(leaf) > 100 {
		leaf = strings.TrimRight(leaf[:100], "-")
	}
	copy := store.Page{Kind: p.Kind, Title: p.Title, Document: p.Document, Translations: p.Translations, Cover: p.Cover, TermIDs: p.TermIDs, SEO: p.SEO, ParentID: p.ParentID, Order: p.Order, Template: p.Template}
	copy.SEO.Canonical = ""
	copy.TermIDs, err = v.Store.ActiveTermIDs(copy.TermIDs)
	if err != nil {
		return copy, err
	}
	for language, tr := range copy.Translations {
		tr.SEO.Canonical = ""
		copy.Translations[language] = tr
	}
	for i := 1; i <= 100; i++ {
		copy.Slug = leaf + "-copy"
		if i > 1 {
			copy.Slug += "-" + strconv.Itoa(i)
		}
		result, e := v.Save(ctx, copy, false)
		if e == nil {
			return result, nil
		}
		if !errors.Is(e, store.ErrConflict) {
			return copy, e
		}
	}
	return copy, store.ErrConflict
}
