package service

import (
	"context"
	"errors"
	"gopress/internal/store"
	"strings"
	"testing"
)

func TestTranslationValidationUsesTheContentBoundary(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	document := `{"type":"doc","content":[{"type":"paragraph"}]}`
	for _, test := range []struct{ language, title, document string }{
		{"../en", "Title", document}, {"en", strings.Repeat("x", 201), document},
		{"en", "Title", `{"type":"doc","content":[{"type":"iframe"}]}`},
		{"en", "Title", `{"type":"doc","content":[{"type":"image","attrs":{"src":"/uploads/` + strings.Repeat("a", 64) + `.png"}}]}`},
	} {
		_, err = v.Save(context.Background(), store.Page{Title: "Base", Slug: "base", Document: document, Translations: map[string]store.Translation{test.language: {Title: test.title, Document: test.document}}}, true)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted translation %+v: %v", test, err)
		}
	}
}

func TestAddedLanguageStaysPrivateUntilWritten(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	document := `{"type":"doc","content":[{"type":"paragraph"}]}`
	p, err := v.Save(context.Background(), store.Page{Title: "Base", Slug: "base", Document: document, SEO: store.SEO{Language: "de"}, Translations: map[string]store.Translation{"en": {Document: document}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.SEO.Language != "de" {
		t.Fatal("source language lost")
	}
	p, err = v.Save(context.Background(), p, true)
	if err != nil {
		t.Fatal(err)
	}
	public, err := s.Public("base")
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Translations) != 0 {
		t.Fatal("published an unfinished translation")
	}
	if len(p.Translations) != 1 {
		t.Fatal("discarded an unfinished translation")
	}
	p.Translations["en"] = store.Translation{Title: "English version", Document: document}
	if _, err = v.Save(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
}

func TestContentTitleLimitCountsUnicodeCharacters(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"type":"doc","content":[{"type":"paragraph"}]}`
	p, err := v.Save(context.Background(), store.Page{Title: strings.Repeat("я", 200), Slug: "unicode", Document: doc, Translations: map[string]store.Translation{"en": {Title: strings.Repeat("語", 200), Document: doc}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	p.Title += "я"
	if _, err = v.Save(context.Background(), p, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted 201 characters", err)
	}
}
