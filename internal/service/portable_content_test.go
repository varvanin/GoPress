package service

import (
	"bytes"
	"context"
	"encoding/json"
	"gopress/internal/store"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableContentRoundTripAndAtomicConflict(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 400, 200))); err != nil {
		t.Fatal(err)
	}
	name, err := v.Upload(bytes.NewReader(imageBytes.Bytes()), ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"type":"doc","content":[{"type":"image","attrs":{"src":"/uploads/` + name + `","alt":"Alt","caption":"Caption"}},{"type":"paragraph","content":[{"type":"text","text":"Keep source-name text","marks":[{"type":"link","attrs":{"href":"/source-two#section"}}]}]}]}`
	first, err := v.Save(ctx, store.Page{Kind: "page", Title: "One", Slug: "source-one", Document: raw, Cover: name, SEO: store.SEO{Canonical: "https://external.test/old"}, Translations: map[string]store.Translation{"en": {Title: "English", Document: raw}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := v.Save(ctx, store.Page{Kind: "post", Title: "Two", Slug: "source-two", Document: `{"type":"doc","content":[{"type":"paragraph"}]}`}, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := v.ExportContent(ctx, []store.Page{first, second})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := InspectContent(data)
	if err != nil || len(bundle.Images) != 1 || len(bundle.Pages) != 2 {
		t.Fatal(bundle, err)
	}
	target := scheduledBackupService(t)
	pages, err := target.ImportContent(ctx, data, "imported")
	if err != nil || len(pages) != 2 {
		t.Fatal(pages, err)
	}
	copy, err := target.Store.Page(pages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Published || copy.ScheduledAt != 0 || copy.Slug != "imported-1" || copy.Cover == name || copy.Cover == "" || copy.SEO.Canonical != "" || !strings.Contains(copy.Document, "/imported-2#section") || !strings.Contains(copy.Document, "Keep source-name text") || copy.Translations["en"].Title != "English" {
		t.Fatal(copy)
	}
	if _, err = os.Stat(filepath.Join(target.Store.Dir, "uploads", copy.Cover)); err != nil {
		t.Fatal(err)
	}
	infos, err := target.Store.ImageInfos([]string{copy.Cover})
	if err != nil || len(infos[copy.Cover].Variants) != 1 {
		t.Fatal(infos, err)
	}
	collision, err := target.Save(ctx, store.Page{Title: "Occupied", Slug: "conflict-2", Document: second.Document}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = collision
	before, err := os.ReadDir(filepath.Join(target.Store.Dir, "uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = target.ImportContent(ctx, data, "conflict"); err == nil {
		t.Fatal("collision accepted")
	}
	var count int
	if err = target.Store.DB.QueryRow(`SELECT count(*) FROM pages WHERE slug='conflict-1'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial content", count, err)
	}
	after, err := os.ReadDir(filepath.Join(target.Store.Dir, "uploads"))
	if err != nil || len(after) != len(before) {
		t.Fatal("media not cleaned up", len(before), len(after), err)
	}
	bundle.Images[0].Name = "../../secret"
	bad, _ := json.Marshal(bundle)
	if _, err = InspectContent(bad); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err = v.ExportContent(ctx, []store.Page{{ID: first.ID, Version: first.Version - 1}}); err == nil {
		t.Fatal("stale export accepted")
	}
	author := store.WithActor(ctx, store.User{ID: 2, Role: "author"})
	if _, err = target.ImportContent(author, data, "author"); err != ErrForbidden {
		t.Fatal("author imported", err)
	}
}
