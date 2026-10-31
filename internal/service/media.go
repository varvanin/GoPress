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
	info := store.ImageInfo{Width: cfg.Width, Height: cfg.Height}
	written := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, file := range written {
				_ = os.Remove(filepath.Join(dir, file))
			}
		}
	}()
	size, err := writeImage(dir, name, img, format)
	if err != nil {
		return "", err
	}
	written = append(written, name)
	for _, target := range []int{320, 960, 1600} {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		if max(cfg.Width, cfg.Height) <= target {
			continue
		}
		resized := resizeImage(img, target)
		variantName := store.ImageVariantName(name, target)
		if _, err = writeImage(dir, variantName, resized, format); err != nil {
			return "", err
		}
		written = append(written, variantName)
		bounds := resized.Bounds()
		info.Variants = append(info.Variants, store.ImageVariant{Size: target, Width: bounds.Dx(), Height: bounds.Dy()})
	}
	if err = v.Store.MediaImage(name, mime, size, store.Owner(ctx), info); err != nil {
		return "", err
	}
	committed = true
	return name, nil
}

func writeImage(dir, name string, img image.Image, format string) (int64, error) {
	f, err := os.CreateTemp(dir, ".upload-")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	// Re-encode pixels to discard metadata and trailing active content.
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
		return 0, err
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return 0, err
	}
	if err = os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return 0, err
	}
	return info.Size(), nil
}
