package service

import (
	"context"
	"gopress/internal/store"
	"strings"
	"unicode/utf8"
)

func (s *Service) SaveFAQ(ctx context.Context, f store.FAQGroup) (int64, error) {
	if err := RequireAdmin(ctx); err != nil {
		return 0, err
	}
	f.Name = strings.TrimSpace(f.Name)
	if f.ID < 0 || f.Version < 0 || f.Name == "" || utf8.RuneCountInString(f.Name) > 120 || len(f.Items) < 1 || len(f.Items) > 50 {
		return 0, ErrInvalid
	}
	for i := range f.Items {
		p := &f.Items[i]
		p.Question = strings.TrimSpace(p.Question)
		p.Answer = strings.TrimSpace(p.Answer)
		if p.Question == "" || p.Answer == "" || utf8.RuneCountInString(p.Question) > 300 || utf8.RuneCountInString(p.Answer) > 5000 {
			return 0, ErrInvalid
		}
	}
	return s.Store.SaveFAQ(ctx, f)
}
