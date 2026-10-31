package service

import (
	"context"
	"gopress/internal/store"
	"strings"
)

func (v *Service) SaveRedirect(ctx context.Context, slug string, id int64) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	slug = strings.TrimPrefix(strings.TrimSpace(slug), "/")
	if len(slug) > 1024 || !validContentPath(slug) || store.ReservedContentPath(slug) || id <= 0 {
		return ErrInvalid
	}
	return v.Store.SaveManualRedirect(ctx, slug, id)
}
func (v *Service) RecordMissing(ctx context.Context, slug string) error {
	if len(slug) > 1024 || !validContentPath(slug) || store.ReservedContentPath(slug) {
		return nil
	}
	return v.Store.RecordMissingPath(ctx, slug)
}
