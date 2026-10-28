package service

import (
	"bytes"
	"context"
	"errors"
	"gopress/internal/store"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

const MaxUpload = 8 << 20

func (v *Service) Upload(r io.Reader, contexts ...context.Context) (string, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	// Bound simultaneous decoder allocations; keep image work outside SQL transactions.
	v.uploadMu.Lock()
	defer v.uploadMu.Unlock()
	data, err := io.ReadAll(io.LimitReader(r, MaxUpload+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 || len(data) > MaxUpload {
		return "", ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return "", ErrInvalid
	}
	img, actual, err := image.Decode(bytes.NewReader(data))
	if err != nil || actual != format {
		return "", ErrInvalid
	}
	ext, mime := ".png", "image/png"
	if format == "jpeg" {
		ext, mime = ".jpg", "image/jpeg"
	}
	name := Token() + ext
	dir := filepath.Join(v.Store.Dir, "uploads")
	f, err := os.CreateTemp(dir, ".upload-")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	// Re-encode decoded pixels to discard metadata and any trailing active content.
	if format == "jpeg" {
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(f, img)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return "", err
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	if err = os.Rename(tmp, dest); err != nil {
		return "", err
	}
	if err = v.Store.Media(name, mime, info.Size(), store.Owner(ctx)); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return name, nil
}
