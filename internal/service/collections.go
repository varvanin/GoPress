package service

import (
	"context"
	"gopress/internal/store"
	"strings"
	"unicode/utf8"
)

func (v *Service) SaveCollection(ctx context.Context, c store.Collection) (int64, error) {
	if err := RequireAdmin(ctx); err != nil {
		return 0, err
	}
	if c.Kind != "gallery" && c.Kind != "testimonials" || c.ID < 0 || c.ID == 0 && c.Version != 0 || c.ID > 0 && c.Version < 1 || c.Layout != "grid" && c.Layout != "list" || c.Kind == "gallery" && c.Layout != "grid" || len(c.Items) < 1 || len(c.Items) > 50 {
		return 0, ErrInvalid
	}
	valid := func(s *string, max int) bool {
		*s = strings.TrimSpace(*s)
		return utf8.ValidString(*s) && utf8.RuneCountInString(*s) <= max
	}
	if !valid(&c.Name, 120) || c.Name == "" {
		return 0, ErrInvalid
	}
	v.mediaMu.RLock()
	defer v.mediaMu.RUnlock()
	for i := range c.Items {
		item := &c.Items[i]
		if !valid(&item.Caption, 500) || !valid(&item.Name, 120) || !valid(&item.Company, 200) || !valid(&item.Text, 5000) {
			return 0, ErrInvalid
		}
		if c.Kind == "gallery" {
			if item.Image == "" {
				return 0, ErrInvalid
			}
			item.Name = ""
			item.Company = ""
			item.Text = ""
		} else {
			if item.Name == "" || item.Text == "" {
				return 0, ErrInvalid
			}
			item.Caption = ""
		}
		if item.Image != "" {
			if !imagePath.MatchString("/uploads/" + item.Image) {
				return 0, ErrInvalid
			}
			if _, err := v.Store.MediaType(item.Image); err != nil {
				return 0, ErrInvalid
			}
		}
	}
	return v.Store.SaveCollection(ctx, c)
}
