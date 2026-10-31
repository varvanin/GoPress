package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopress/internal/store"
)

func (v *Service) SaveMediaDetails(f store.MediaFile) error {
	f.Title = strings.TrimSpace(f.Title)
	f.Alt = strings.TrimSpace(f.Alt)
	f.Caption = strings.TrimSpace(f.Caption)
	if !utf8.ValidString(f.Title+f.Alt+f.Caption) || len(f.Title) > 200 || len(f.Alt) > 500 || len(f.Caption) > 1000 {
		return ErrInvalid
	}
	return v.Store.SaveMediaDetails(f)
}
func (v *Service) DeleteMedia(ctx context.Context, name string) error {
	if filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || name == "." {
		return ErrInvalid
	}
	v.mediaMu.Lock()
	defer v.mediaMu.Unlock()
	// Commit the reference removal first. An interrupted unlink can only leave an unserved orphan.
	if err := v.Store.DeleteMedia(ctx, name); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return store.ErrMediaUsed
		}
		return err
	}
	var cleanup error
	for _, file := range []string{name, store.ImageVariantName(name, 320), store.ImageVariantName(name, 960), store.ImageVariantName(name, 1600)} {
		if e := os.Remove(filepath.Join(v.Store.Dir, "uploads", file)); e != nil && !errors.Is(e, os.ErrNotExist) {
			cleanup = errors.Join(cleanup, e)
		}
	}
	return cleanup
}

func (v *Service) SaveMenu(ctx context.Context, m store.Menu) error {
	if m.Language != "" && !ValidContentLanguage(m.Language) {
		return ErrInvalid
	}
	if (m.Position != "header" && m.Position != "footer") || m.Version < 0 || len(m.Items) > 50 {
		return ErrInvalid
	}
	depth := -1
	for i := range m.Items {
		item := &m.Items[i]
		item.Label = strings.TrimSpace(item.Label)
		item.URL = strings.TrimSpace(item.URL)
		if item.Label == "" || len(item.Label) > 120 || !utf8.ValidString(item.Label) || len(item.URL) > 2048 || !SafeLink(item.URL) || item.Depth < 0 || item.Depth > 2 || item.Depth > depth+1 {
			return ErrInvalid
		}
		depth = item.Depth
	}
	if m.Items == nil {
		m.Items = []store.MenuItem{}
	}
	return v.Store.SaveMenu(ctx, m)
}
