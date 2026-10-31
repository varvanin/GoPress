package service

import (
	"context"
	"gopress/internal/store"
	"strings"
	"unicode/utf8"
)

func (v *Service) RequestReview(ctx context.Context, id, version int64) error {
	actor, ok := store.Actor(ctx)
	if !ok || actor.ID <= 0 {
		return ErrUnauthorized
	}
	p, err := v.Store.Page(id)
	if err != nil {
		return err
	}
	if !CanEdit(ctx, p) {
		return ErrForbidden
	}
	return v.Store.RequestReview(ctx, id, version)
}
func (v *Service) ResolveReview(ctx context.Context, id, version int64, publish bool, comment string) error {
	if !CanPublish(ctx) {
		return ErrForbidden
	}
	comment = strings.TrimSpace(comment)
	if !utf8.ValidString(comment) || len(comment) > 4000 || utf8.RuneCountInString(comment) > 1000 || !publish && comment == "" {
		return ErrInvalid
	}
	return v.Store.ResolveReview(ctx, id, version, publish, comment)
}
