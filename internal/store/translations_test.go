package store

import (
	"context"
	"errors"
	"testing"
)

func TestTranslationsStayWithPublicationAndScheduledRevision(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	p, err := s.Save(ctx, Page{Title: "Original", Slug: "about", SEO: SEO{Language: "de"}, Document: `{"type":"doc"}`, Translations: map[string]Translation{"en": {Title: "Published", Document: `{"type":"doc"}`}, "pl": {Document: `{"type":"doc"}`}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	publishedVersion := p.Version
	p.Translations["en"] = Translation{Title: "Scheduled", Document: `{"type":"doc"}`}
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	public, err := s.Public("about")
	if err != nil || public.Translations["en"].Title != "Published" {
		t.Fatal(public, err)
	}
	if err = s.Schedule(ctx, p.ID, p.Version, 100); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.SEO.Language = "it"
	p.Translations["en"] = Translation{Title: "Later private draft", Document: `{"type":"doc"}`}
	p, err = s.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	stale := p
	stale.Version--
	if _, err = s.Save(ctx, stale, true); !errors.Is(err, ErrConflict) {
		t.Fatal("stale translation save", err)
	}
	if _, err = s.PublishDue(ctx, 101); err != nil {
		t.Fatal(err)
	}
	public, err = s.Public("about")
	if err != nil || public.Translations["en"].Title != "Scheduled" || public.SEO.Language != "de" {
		t.Fatal(public, err)
	}
	if _, exists := public.Translations["pl"]; exists {
		t.Fatal("scheduled publication exposed an empty added version")
	}
	original, err := s.Revision(p.ID, publishedVersion)
	if err != nil || original.Translations["en"].Title != "Published" {
		t.Fatal(original, err)
	}
	draft, err := s.Page(p.ID)
	if err != nil || draft.Translations["en"].Title != "Later private draft" {
		t.Fatal(draft, err)
	}
}

func TestTranslationImagesRemainProtectedByRevisions(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	name := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png"
	if _, err = s.DB.Exec("INSERT INTO media(name,mime,size) VALUES(?,'image/png',1)", name); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, err := s.Save(ctx, Page{Title: "Original", Slug: "image", Document: `{"type":"doc"}`, Translations: map[string]Translation{"en": {Title: "English", Document: `{"type":"doc","content":[{"type":"image","attrs":{"src":"/uploads/` + name + `"}}]}`}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	used, err := s.MediaUsed(name)
	if err != nil || !used {
		t.Fatal("translation image unprotected", err)
	}
	p.Translations = map[string]Translation{}
	if _, err = s.Save(ctx, p, true); err != nil {
		t.Fatal(err)
	}
	used, err = s.MediaUsed(name)
	if err != nil || !used {
		t.Fatal("revision image unprotected", err)
	}
}
