package httpui

import (
	"bytes"
	"context"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gopress/internal/store"
)

func TestThemeHierarchyRawFilesPartialsAndEscaping(t *testing.T) {
	source := fstest.MapFS{
		"layout.html":     {Data: []byte(`{{template "header" .}}<main>{{template "body" .}}</main>{{template "footer" .}}`)},
		"header.html":     {Data: []byte(`<header>{{.Site.Title}}<link href="{{asset .ThemeAssets "images/logo.svg"}}"></header>`)},
		"footer.html":     {Data: []byte(`<footer>{{.Year}}</footer>`)},
		"parts/card.html": {Data: []byte(`<b>{{.Title}}</b>`)},
		"page.html":       {Data: []byte(`PAGE {{.Body}} {{range .Posts}}{{template "parts/card" .}}{{end}}`)},
		"home.html":       {Data: []byte(`HOME`)},
		"single.html":     {Data: []byte(`SINGLE`)},
		"archive.html":    {Data: []byte(`ARCHIVE`)},
		"search.html":     {Data: []byte(`SEARCH`)},
		"404.html":        {Data: []byte(`NOTFOUND`)},
	}
	for page, want := range map[string]string{"page.html": "PAGE", "home.html": "HOME", "single.html": "SINGLE", "archive.html": "ARCHIVE", "search.html": "SEARCH", "404.html": "NOTFOUND"} {
		parsed, err := themeTemplates(source, "", page, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = parsed.ExecuteTemplate(&out, "layout", ThemeData{Site: ThemeSite{Title: `<script>bad</script>`}, ThemeAssets: "/theme-assets/test", Year: 2026, Body: template.HTML("<p>safe body</p>"), Posts: []store.Page{{Title: "<unsafe>"}}})
		if err != nil {
			t.Fatal(err)
		}
		html := out.String()
		if !strings.Contains(html, want) || !strings.Contains(html, "/theme-assets/test/images/logo.svg") || strings.Contains(html, "<script>") || !strings.Contains(html, "<footer>2026</footer>") {
			t.Fatal(html)
		}
		if page == "page.html" && (!strings.Contains(html, "<p>safe body</p>") || !strings.Contains(html, "&lt;unsafe&gt;")) {
			t.Fatal(html)
		}
	}
	delete(source, "single.html")
	delete(source, "archive.html")
	delete(source, "search.html")
	delete(source, "404.html")
	for requested, want := range map[string]string{"single.html": "page.html", "archive.html": "home.html", "search.html": "page.html", "404.html": "page.html"} {
		got, err := resolveThemePage(source, "", requested)
		if err != nil || got != want {
			t.Fatal(requested, got, err)
		}
	}
	delete(source, "home.html")
	if got, err := resolveThemePage(source, "", "archive.html"); err != nil || got != "page.html" {
		t.Fatal(got, err)
	}
	source["layout.html"] = &fstest.MapFile{Data: []byte(`{{define "layout"}}{{template "body" .}}{{end}}{{define "body"}}OLD DEFAULT{{end}}`)}
	parsed, err := themeTemplates(source, "", "page.html", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = parsed.ExecuteTemplate(&out, "layout", ThemeData{}); err != nil || strings.Contains(out.String(), "OLD DEFAULT") || !strings.Contains(out.String(), "PAGE") {
		t.Fatal(out.String(), err)
	}
	source["page.html"] = &fstest.MapFile{Data: []byte("<h1>Title</h1>\n{{if}}")}
	if _, err := themeTemplates(source, "", "single.html", time.UTC); err == nil || !strings.Contains(err.Error(), "page.html:2") {
		t.Fatal("missing syntax location", err)
	}
}

func TestThemePaginationPreservesOnlyPublicFilters(t *testing.T) {
	p := themePagination("/search", url.Values{"q": {"a & b"}, "token": {"secret"}}, 2, 61, 20)
	if p.Current != 2 || p.TotalPages != 4 || p.PreviousURL != "/search?q=a+%26+b" || p.NextURL != "/search?page=3&q=a+%26+b" {
		t.Fatal(p)
	}
	p = themePagination("/", url.Values{"term": {"7"}}, 1, 0, 12)
	if p.PreviousURL != "" || p.NextURL != "" || p.TotalPages != 0 {
		t.Fatal(p)
	}
}

func TestThemeValidationNamesBadFilesBeforeActivation(t *testing.T) {
	_, app, _ := seoApp(t)
	for name, content := range map[string]string{
		"single.html":    "<h1>Test</h1>\n{{.MissingField}}",
		"parts/bad.html": "<p>Test</p>\n{{if}}",
		"landing.html":   "<h1>Test</h1>\n{{template \"missing-part\" .}}",
	} {
		files := themeFiles()
		files[name] = content
		_, err := app.installTheme(bytes.NewReader(themeZip(t, files)))
		if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), ":2") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	files := themeFiles()
	files["page.html"] = `{{asset .ThemeAssets "../private"}}`
	if _, err := app.installTheme(bytes.NewReader(themeZip(t, files))); err == nil || !strings.Contains(err.Error(), "asset") {
		t.Fatal(err)
	}
}

func TestThemeRoutesChooseNewFilesAndKeepFallbackStatuses(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	files := themeFiles()
	for name, marker := range map[string]string{"single.html": "SINGLE ROUTE", "archive.html": "ARCHIVE ROUTE", "search.html": "SEARCH ROUTE"} {
		files[name] = `<h2>` + marker + `</h2>{{.Body}}`
	}
	files["landing.html"] = `<h2>LANDING ROUTE</h2>{{.Title}}`
	files["header.html"] = `<header>SHARED HEADER</header>`
	files["footer.html"] = `<footer>SHARED FOOTER</footer>`
	files["layout.html"] = `{{template "header" .}}{{template "body" .}}{{template "footer" .}}`
	delete(files, "404.html")
	if _, err := app.installTheme(bytes.NewReader(themeZip(t, files))); err != nil {
		t.Fatal(err)
	}
	appearance, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = "theme:test-theme"
	if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
		t.Fatal(err)
	}
	for _, p := range []store.Page{{Title: "Public page", Slug: "theme-page", Kind: "page", Document: document}, {Title: "Public post", Slug: "theme-post", Kind: "post", Document: document}, {Title: "Second post", Slug: "second-post", Kind: "post", Document: document}, {Title: "Landing", Slug: "theme-landing", Kind: "page", Template: "landing.html", Document: document}} {
		if _, err = app.Service.Save(ctx, p, true); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.PostsPerPage = 1
	if err = app.Service.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	for route, want := range map[string]string{"/": "MY CUSTOM HOME", "/theme-page": "Public page", "/theme-landing": "LANDING ROUTE", "/theme-post": "SINGLE ROUTE", "/?page=2": "ARCHIVE ROUTE", "/search?q=Public": "SEARCH ROUTE", "/missing-page": "Страница не найдена"} {
		result := b.request("GET", route, "", "", false)
		expected := 200
		if route == "/missing-page" {
			expected = 404
		}
		status(t, result, expected)
		if !strings.Contains(result.Body.String(), want) || !strings.Contains(result.Body.String(), "SHARED HEADER") || !strings.Contains(result.Body.String(), "SHARED FOOTER") {
			t.Fatal(route, result.Body.String())
		}
	}
}

func TestActivationShowsFileAndLineWithoutChangingTheme(t *testing.T) {
	s, app, b := seoApp(t)
	if _, err := app.installTheme(bytes.NewReader(themeZip(t, themeFiles()))); err != nil {
		t.Fatal(err)
	}
	before, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.Dir, "theme", "packages", "test-theme", "single.html")
	if err = os.WriteFile(file, []byte("<h1>Broken</h1>\n{{.UnknownField}}"), 0600); err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"scope": {"theme"}, "intent": {"activate"}, "version": {strconv.FormatInt(before.Version, 10)}, "theme": {"theme:test-theme"}}
	response := adminHTML(b, "POST", "/admin/themes", fields, true, false, true)
	status(t, response, 400)
	if !strings.Contains(response.Body.String(), "single.html:2") {
		t.Fatal("missing error location", response.Body.String())
	}
	after, err := s.Appearance()
	if err != nil || after.Theme != before.Theme || after.Version != before.Version {
		t.Fatal(after, err)
	}
}

func TestThemeExecutionStopsOversizedOutputAndEmptyLoops(t *testing.T) {
	for _, tc := range []struct {
		name, layout, want string
		posts              int
	}{
		{"oversized", strings.Repeat("x", (8<<20)+1), "8 MiB", 0},
		{"recursive", `{{define "repeat"}}` + strings.Repeat("x", 1024) + `{{template "repeat" .}}{{end}}{{template "repeat" .}}`, "8 MiB", 0},
		{"empty loops", `{{range .Posts}}{{range $.Posts}}{{range $.Posts}}{{end}}{{end}}{{end}}`, "100000 steps", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fstest.MapFS{"layout.html": {Data: []byte(tc.layout)}, "page.html": {Data: []byte("Page")}}
			compiled, err := themeTemplates(source, "", "page.html", time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			var output themeOutput
			data := ThemeData{Posts: make([]store.Page, tc.posts)}
			if err = executeTheme(compiled, &output, data); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("execution was not bounded", err)
			}
			if output.Len() > 8<<20 {
				t.Fatal("output exceeded limit", output.Len())
			}
			if tc.name == "oversized" {
				if err = validateThemeFiles(source, "", store.Appearance{}); err == nil || !strings.Contains(err.Error(), "2 MiB") {
					t.Fatal("oversized template passed validation", err)
				}
			}
		})
	}
}

func TestThemePublicBudgetAllowsFullEscapedDocuments(t *testing.T) {
	source := fstest.MapFS{"layout.html": {Data: []byte(`{{.Body}}`)}, "page.html": {Data: []byte("Page")}}
	compiled, err := themeTemplates(source, "", "page.html", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("&lt;", 600000)
	var output themeOutput
	if err = executeTheme(compiled, &output, ThemeData{Body: template.HTML(body)}); err != nil || output.String() != body {
		t.Fatal("valid escaped document was rejected", err)
	}
}
