package httpui

import (
	"bytes"
	"context"
	"fmt"
	"gopress/internal/store"
	"image"
	"image/png"
	"io/fs"
	"strings"
	"testing"
)

func TestMinimalPublicationStatesAndNavigation(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	appearance, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = "minimal"
	if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
		t.Fatal(err)
	}
	empty := b.request("GET", "/", "", "", false)
	status(t, empty, 200)
	for _, want := range []string{`class="minimal-sidebar"`, `class="minimal-mobile"`, "Пока чистый лист."} {
		if !strings.Contains(empty.Body.String(), want) {
			t.Fatal("empty publication missing", want)
		}
	}
	var last store.Page
	for i := 1; i <= 4; i++ {
		last, err = app.Service.Save(ctx, store.Page{Kind: "post", Title: fmt.Sprintf("Minimal story %d", i), Slug: fmt.Sprintf("minimal-%d", i), Document: document, SEO: store.SEO{Description: "A published excerpt"}}, true)
		if err != nil {
			t.Fatal(err)
		}
		home := b.request("GET", "/", "", "", false)
		status(t, home, 200)
		html := home.Body.String()
		if strings.Count(html, `class="post-card"`) != i || strings.Contains(html, "<img") || strings.Contains(html, "<script") {
			t.Fatal("feed must remain text-first", i)
		}
		for n := 1; n <= i; n++ {
			if strings.Count(html, ">"+fmt.Sprintf("Minimal story %d", n)+"</span>") != 1 {
				t.Fatal("missing or repeated title", n)
			}
		}
	}
	last.Title = "PRIVATE MINIMAL DRAFT"
	if _, err = app.Service.Save(ctx, last, false); err != nil {
		t.Fatal(err)
	}
	article := b.request("GET", "/minimal-4", "", "", false)
	status(t, article, 200)
	for _, want := range []string{`class="minimal-reading article-content"`, "мин чтения", "Продолжить чтение"} {
		if !strings.Contains(article.Body.String(), want) {
			t.Fatal(want)
		}
	}
	if strings.Contains(article.Body.String(), "PRIVATE MINIMAL DRAFT") || strings.Contains(article.Body.String(), `class="minimal-cover"`) {
		t.Fatal("draft or artificial cover appeared")
	}
	if _, err = app.Service.Save(ctx, store.Page{Title: "About this site", Slug: "about-minimal", Document: document}, true); err != nil {
		t.Fatal(err)
	}
	if err = app.Service.SaveMenu(ctx, store.Menu{Position: "header", Items: []store.MenuItem{{Label: "Записи", URL: "/", Depth: 0}, {Label: "О сайте", URL: "/about-minimal", Depth: 0}, {Label: "Вложенный раздел", URL: "/about-minimal", Depth: 1}, {Label: "Третий уровень", URL: "/about-minimal", Depth: 2}}}); err != nil {
		t.Fatal(err)
	}
	page := b.request("GET", "/about-minimal", "", "", false)
	status(t, page, 200)
	for _, want := range []string{"Вложенный раздел", "Третий уровень", `aria-label="Открыть меню"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatal("navigation missing", want)
		}
	}
	if strings.Contains(page.Body.String(), "мин чтения") || strings.Contains(page.Body.String(), "minimal-more") {
		t.Fatal("ordinary page has article-only sections")
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
	if !strings.Contains(archive.Body.String(), "minimal-archive") || strings.Count(archive.Body.String(), `class="post-card"`) != 1 || !strings.Contains(archive.Body.String(), "minimal-pagination") {
		t.Fatal("invalid archive")
	}
	search := b.request("GET", "/search?q=Minimal", "", "", false)
	status(t, search, 200)
	for _, want := range []string{"minimal-search-form", "minimal-results", "Найдено: 4"} {
		if !strings.Contains(search.Body.String(), want) {
			t.Fatal(want)
		}
	}
	noResults := b.request("GET", "/search?q=unfindableword", "", "", false)
	status(t, noResults, 200)
	if !strings.Contains(noResults.Body.String(), "Другие слова — другой результат.") {
		t.Fatal("missing search empty state")
	}
	missing := b.request("GET", "/missing-minimal", "", "", false)
	status(t, missing, 404)
	if !strings.Contains(missing.Body.String(), "minimal-not-found") {
		t.Fatal("missing designed 404")
	}
}

func TestMinimalKeepsUploadedCoverInsideArticle(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	appearance, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = "minimal"
	if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err = png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 24, 24))); err != nil {
		t.Fatal(err)
	}
	name, err := app.Service.Upload(&data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.Service.Save(ctx, store.Page{Kind: "post", Title: "Covered post", Slug: "covered-minimal", Document: document, Cover: name}, true); err != nil {
		t.Fatal(err)
	}
	home := b.request("GET", "/", "", "", false)
	status(t, home, 200)
	if strings.Contains(home.Body.String(), "<img") {
		t.Fatal("cover should not clutter text feed")
	}
	article := b.request("GET", "/covered-minimal", "", "", false)
	status(t, article, 200)
	if !strings.Contains(article.Body.String(), `src="/uploads/`+name+`"`) {
		t.Fatal("uploaded cover disappeared")
	}
}

func TestMinimalCanBeInstalledAsTemplatePackage(t *testing.T) {
	s, app, b := seoApp(t)
	preview, err := resources.ReadFile("assets/template-minimal.svg")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"theme.json": `{"id":"minimal-export","name":"Minimal export","version":"1.0"}`, "screenshot.svg": string(preview)}
	err = fs.WalkDir(resources, "theme-minimal", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := resources.ReadFile(name)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(name, "theme-minimal/")] = strings.ReplaceAll(string(raw), "/assets/public-minimal.css", `{{asset .ThemeAssets "public-minimal.css"}}`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := resources.ReadFile("assets/public-minimal.css")
	if err != nil {
		t.Fatal(err)
	}
	files["assets/public-minimal.css"] = string(css)
	info, err := app.installTheme(bytes.NewReader(themeZip(t, files)))
	if err != nil {
		t.Fatal(err)
	}
	appearance, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = info.ID
	if err = app.Service.SaveAppearance(context.Background(), appearance); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), `href="/theme-assets/minimal-export/public-minimal.css"`) || !strings.Contains(response.Body.String(), `data-theme="theme:minimal-export"`) {
		t.Fatal("export lost its asset paths or identity")
	}
	status(t, b.request("GET", "/theme-assets/minimal-export/public-minimal.css", "", "", false), 200)
}
