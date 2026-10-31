package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gopress/internal/store"
)

func TestResizePreservesBoundsAspectAndPremultipliedAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 8, 805, 408))
	for y := 8; y < 408; y++ {
		for x := 5; x < 805; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
		}
	}
	dst := resizeImage(src, 320)
	if dst.Bounds() != image.Rect(0, 0, 320, 160) {
		t.Fatal(dst.Bounds())
	}
	r, g, b, a := dst.At(100, 80).RGBA()
	if a < 32000 || a > 33000 || r > 26000 || r < 25000 || g < 12000 || g > 14000 || b < 6000 || b > 7000 {
		t.Fatal("transparent edge channels changed", r, g, b, a)
	}
	tall := resizeImage(image.NewRGBA(image.Rect(0, 0, 20, 2000)), 320)
	if tall.Bounds() != image.Rect(0, 0, 3, 320) {
		t.Fatal(tall.Bounds())
	}
}
func TestImageVariantsRegisterAtomicallyAndDeleteTogether(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	if err = png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	name, err := v.Upload(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	infos, err := s.ImageInfos([]string{name})
	if err != nil {
		t.Fatal(err)
	}
	info := infos[name]
	if info.Width != 1200 || info.Height != 600 || len(info.Variants) != 2 || info.Variants[0].Width != 320 || info.Variants[1].Width != 960 {
		t.Fatal(info)
	}
	for _, variant := range info.Variants {
		file, err := os.Open(filepath.Join(s.Dir, "uploads", store.ImageVariantName(name, variant.Size)))
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(file)
		file.Close()
		if err != nil || cfg.Width != variant.Width || cfg.Height != variant.Height {
			t.Fatal(cfg, variant, err)
		}
	}
	if err = v.DeleteMedia(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{name, store.ImageVariantName(name, 320), store.ImageVariantName(name, 960)} {
		if _, err = os.Stat(filepath.Join(s.Dir, "uploads", file)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("image remains", file, err)
		}
	}
	infos, err = s.ImageInfos([]string{name})
	if err != nil || len(infos) != 0 {
		t.Fatal("metadata remains", infos, err)
	}
}
func TestImageUploadFailureLeavesNoOriginalOrVariants(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`CREATE TRIGGER fail_image_metadata BEFORE INSERT ON media_images BEGIN SELECT RAISE(ABORT,'test failure'); END;`); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	_ = png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 640, 320)))
	if _, err = v.Upload(bytes.NewReader(data.Bytes())); err == nil {
		t.Fatal("accepted failed registration")
	}
	files, err := os.ReadDir(filepath.Join(s.Dir, "uploads"))
	if err != nil || len(files) != 0 {
		t.Fatal("unregistered files remain", files, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT count(*) FROM media`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial database registration", count, err)
	}
}
