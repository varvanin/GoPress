package httpui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestPublicLanguageSnapshotsSearchMenusAndSEO(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Title: "Original", Slug: "languages", Document: document, SEO: store.SEO{Language: "de"}, Translations: map[string]store.Translation{
		"en": {Title: "English uniqueheadline", Document: document},
		"pl": {Title: "Polish privateindex", Document: document, SEO: store.SEO{NoIndex: true}},
	}}, true)
	if err != nil {
		t.Fatal(err)
	}
	menu := store.Menu{Position: "header", Language: "en", Items: []store.MenuItem{{Label: "English navigation", URL: "/languages?lang=en"}}}
	if err = app.Service.SaveMenu(ctx, menu); err != nil {
		t.Fatal(err)
	}
	if err = app.Service.SaveMenu(ctx, menu); err != store.ErrConflict {
		t.Fatal("stale localized menu overwrote current menu", err)
	}
	saved, err := s.Menu("header", "en")
	if err != nil || saved.Version != 1 {
		t.Fatal(saved, err)
	}
	saved.Items[0].Label = "English navigation updated"
	if err = app.Service.SaveMenu(ctx, saved); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"journal", "minimal", "studio"} {
		appearance, e := s.Appearance()
		if e != nil {
			t.Fatal(e)
		}
		appearance.Theme = theme
		if e = app.Service.SaveAppearance(ctx, appearance); e != nil {
			t.Fatal(e)
		}
		r := b.request("GET", "/languages?lang=en", "", "", false)
		status(t, r, 200)
		body := r.Body.String()
		for _, want := range []string{`lang="en"`, `hreflang="de"`, `hreflang="en"`, `/languages?lang=en`, `English uniqueheadline`, `English navigation updated`} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %s", theme, want)
			}
		}
		if strings.Contains(body, `<link rel="alternate" hreflang="pl"`) {
			t.Fatal("noindex language in alternates")
		}
		status(t, b.request("GET", "/languages?lang=de", "", "", false), 200)
	}
	r := b.request("GET", "/search?q=uniqueheadline&lang=en", "", "", false)
	status(t, r, 200)
	if !strings.Contains(r.Body.String(), "English uniqueheadline") || !strings.Contains(r.Body.String(), `/languages?lang=en`) {
		t.Fatal("translated search missing", r.Body.String())
	}
	r = b.request("GET", "/search?q=uniqueheadline&lang=de", "", "", false)
	status(t, r, 200)
	if strings.Contains(r.Body.String(), "English uniqueheadline") {
		t.Fatal("language filter ignored")
	}
	r = b.request("GET", "/sitemap.xml", "", "", false)
	status(t, r, 200)
	if !strings.Contains(r.Body.String(), "/languages?lang=en") || strings.Contains(r.Body.String(), "/languages?lang=pl") {
		t.Fatal("wrong translated sitemap", r.Body.String())
	}
	p.Translations["en"] = store.Translation{Title: "Private changedheadline", Document: document}
	p, err = app.Service.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	results, _, err := s.SearchPublic("uniqueheadline", 1, "en")
	if err != nil || len(results) != 1 {
		t.Fatal("draft replaced published search", results, err)
	}
	delete(p.Translations, "en")
	p, err = app.Service.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", "/languages?lang=en", "", "", false), 200)
	if err = s.Publish(ctx, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", "/languages?lang=en", "", "", false), 404)
	if err = s.RebuildSearch(); err != nil {
		t.Fatal(err)
	}
	results, _, err = s.SearchPublic("uniqueheadline", 1, "en")
	if err != nil || len(results) != 0 {
		t.Fatal("removed translation remains indexed", results, err)
	}
	edit := b.request("GET", fmt.Sprintf("/admin/pages/%d/edit", p.ID), "", "", false)
	status(t, edit, 200)
	if !strings.Contains(edit.Body.String(), `name="original_content_language"`) {
		t.Fatal("original language control missing")
	}
}

func TestLocalizedContentListsDoNotLoadTranslationDocuments(t *testing.T) {
	s, app, _ := seoApp(t)
	for _, kind := range []string{"page", "post"} {
		_, err := app.Service.Save(context.Background(), store.Page{Kind: kind, Title: "Base", Slug: "list-" + kind, Document: document, Translations: map[string]store.Translation{"en": {Title: "English list", Document: document}}}, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	pages, err := s.PublishedContent("page", "en")
	if err != nil || len(pages) != 1 || pages[0].Title != "English list" || pages[0].URL != "/list-page?lang=en" || pages[0].Document != "" {
		t.Fatal(pages, err)
	}
	posts, _, err := s.PublicPostsLanguage(0, 1, 12, "en")
	if err != nil || len(posts) != 1 || posts[0].Title != "English list" || posts[0].URL != "/list-post?lang=en" || posts[0].Document != "" {
		raw, _ := json.Marshal(posts)
		t.Fatal(string(raw), err)
	}
}

func TestLocalizedPostLinksToAvailableStaticHomeLanguage(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	home, err := app.Service.Save(ctx, store.Page{Title: "Home", Slug: "home-page", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.Service.Save(ctx, store.Page{Kind: "post", Title: "Story", Slug: "translated-story", Document: document, Translations: map[string]store.Translation{"en": {Title: "English story", Document: document}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.HomeMode = "page"
	settings.HomePageID = home.ID
	if err = s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/translated-story?lang=en", "", "", false)
	status(t, response, 200)
	if strings.Contains(response.Body.String(), `href="/?lang=en"`) {
		t.Fatal("home link leads to an unavailable translation")
	}
	status(t, b.request("GET", "/", "", "", false), 200)
}
