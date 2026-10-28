package httpui

import (
	"context"
	"fmt"
	"gopress/internal/store"
	"strings"
	"testing"
)

func TestJournalEditorialStatesAndAccessibleLinks(t *testing.T) {
	s, app, b := seoApp(t)
	empty := b.request("GET", "/", "", "", false)
	status(t, empty, 200)
	if !strings.Contains(empty.Body.String(), "Первая история ещё впереди.") {
		t.Fatal("missing empty state")
	}
	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		p := store.Page{Kind: "post", Title: fmt.Sprintf("Journal story %d", i), Slug: fmt.Sprintf("journal-%d", i), Document: document, SEO: store.SEO{Description: "A public description"}}
		if _, err := app.Service.Save(ctx, p, true); err != nil {
			t.Fatal(err)
		}
		response := b.request("GET", "/", "", "", false)
		status(t, response, 200)
		html := response.Body.String()
		if strings.Count(html, `class="featured-story"`) != 1 {
			t.Fatal("expected one lead story")
		}
		for n := 1; n <= i; n++ {
			title := fmt.Sprintf("Journal story %d", n)
			if strings.Count(html, ">"+title+"</a>") != 1 {
				t.Fatal("missing or repeated story", i, title)
			}
		}
		if !strings.Contains(html, `aria-label="Поиск по журналу"`) || !strings.Contains(html, `aria-label="Главная история:`) {
			t.Fatal("cover/search links need accessible names")
		}
	}
	article := b.request("GET", "/journal-1", "", "", false)
	status(t, article, 200)
	for _, want := range []string{`class="article-layout"`, `class="article-rail"`, `class="article-content"`, "Ещё почитать", "journal-cover-"} {
		if !strings.Contains(article.Body.String(), want) {
			t.Fatal("article missing", want)
		}
	}
	if _, err := app.Service.Save(ctx, store.Page{Title: "Plain page", Slug: "plain-page", Document: document}, true); err != nil {
		t.Fatal(err)
	}
	page := b.request("GET", "/plain-page", "", "", false)
	status(t, page, 200)
	if strings.Contains(page.Body.String(), `class="article-rail"`) || strings.Contains(page.Body.String(), `class="page-cover"`) {
		t.Fatal("ordinary pages should not gain a fake article cover or reading rail")
	}
	settings, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.PostsPerPage = 1
	if err = app.Service.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	archive := b.request("GET", "/?page=2", "", "", false)
	status(t, archive, 200)
	if strings.Contains(archive.Body.String(), `class="featured-story"`) || !strings.Contains(archive.Body.String(), `post-grid archive-grid`) {
		t.Fatal("archive must use its own layout")
	}
	search := b.request("GET", "/search?q=Journal", "", "", false)
	status(t, search, 200)
	for _, want := range []string{`class="journal-search"`, `class="search-results"`, "Найдено: 4"} {
		if !strings.Contains(search.Body.String(), want) {
			t.Fatal("search missing", want)
		}
	}
	missing := b.request("GET", "/missing-journal-page", "", "", false)
	status(t, missing, 404)
	if strings.Contains(missing.Body.String(), `class="reader-subscribe"`) {
		t.Fatal("404 should concentrate on recovery")
	}
}

func TestJournalCoverAssetsAndUploadedCoverPriority(t *testing.T) {
	if got := journalCover("anything", "uploaded.png"); got != "/uploads/uploaded.png" {
		t.Fatal(got)
	}
	for i := 0; i < 20; i++ {
		got := journalCover(fmt.Sprint(i), "")
		if got != journalCover(fmt.Sprint(i), "") {
			t.Fatal("unstable cover")
		}
		raw, err := resources.ReadFile(strings.TrimPrefix(got, "/"))
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 350000 || len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
			t.Fatal("invalid or oversized cover", got, len(raw))
		}
	}
}
