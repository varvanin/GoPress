package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestMediaLibraryProtectsSnapshotsAndDeletesUnused(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	var imageData bytes.Buffer
	if err = png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	used, err := v.Upload(bytes.NewReader(imageData.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	spare, err := v.Upload(bytes.NewReader(imageData.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	details, err := s.MediaFile(used)
	if err != nil {
		t.Fatal(err)
	}
	details.Title, details.Alt, details.Caption = "Обложка 100%", "Пейзаж", "Описание"
	if err = v.SaveMediaDetails(details); err != nil {
		t.Fatal(err)
	}
	files, total, err := s.ListMedia("ОБЛОЖКА", 1)
	if err != nil || total != 1 || files[0].Alt != "Пейзаж" {
		t.Fatalf("search %+v %d %v", files, total, err)
	}
	_, total, err = s.ListMedia("100%", 1)
	if err != nil || total != 1 {
		t.Fatalf("literal search %d %v", total, err)
	}
	ctx := context.Background()
	p, err := v.Save(ctx, store.Page{Title: "Image", Slug: "image", Document: `{"type":"doc","content":[{"type":"image","attrs":{"src":"/uploads/` + used + `","alt":"","caption":""}}]}`}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.Document = `{"type":"doc","content":[{"type":"paragraph"}]}`
	if _, err = v.Save(ctx, p, true); err != nil {
		t.Fatal(err)
	}
	if err = v.DeleteMedia(ctx, used); !errors.Is(err, store.ErrMediaUsed) {
		t.Fatalf("deleted revision image: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.Dir, "uploads", used)); err != nil {
		t.Fatal(err)
	}
	if err = v.DeleteMedia(ctx, spare); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MediaType(spare); err == nil {
		t.Fatal("media row remains")
	}
	if _, err = os.Stat(filepath.Join(s.Dir, "uploads", spare)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file remains", err)
	}
	if err = v.DeleteMedia(ctx, "../site.db"); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted traversal")
	}
}
func TestMenuValidationAndConcurrency(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m := store.Menu{Position: "header", Items: []store.MenuItem{{Label: "Home", URL: "/"}, {Label: "Child", URL: "/child", Depth: 1}, {Label: "Contact", URL: "https://example.com"}}}
	if err = v.SaveMenu(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err = v.SaveMenu(ctx, m); !errors.Is(err, store.ErrConflict) {
		t.Fatal("lost stale update", err)
	}
	got, err := s.Menu("header")
	if err != nil || got.Version != 1 || len(got.Items) != 3 || got.Items[1].Depth != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	for _, item := range []store.MenuItem{{Label: "Bad", URL: "javascript:alert(1)"}, {Label: "Bad", URL: "//evil.test"}, {Label: "Bad", URL: "/", Depth: 1}, {Label: strings.Repeat("a", 121), URL: "/"}} {
		got.Items = []store.MenuItem{item}
		if err = v.SaveMenu(ctx, got); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", item, err)
		}
	}
	got.Items = nil
	if err = v.SaveMenu(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = s.Menu("header")
	if err != nil || len(got.Items) != 0 || got.Version != 2 {
		t.Fatalf("clear %+v %v", got, err)
	}
}
