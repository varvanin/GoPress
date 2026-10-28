package httpui

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/store"
)

func seoApp(t *testing.T) (*store.Store, *App, *browser) {
	t.Helper()
	s, app := openApp(t, t.TempDir())
	t.Cleanup(func() { s.Close() })
	if err := app.Service.Install(context.Background(), app.Service.InstallToken, store.Site{Title: "Test & Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	return s, app, b
}
func TestPublicSEOAndFeedsUsePublishedSnapshots(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Kind: "post", Slug: "story", Title: "Story & <news>", Document: document, SEO: store.SEO{Title: `SEO "title"`, Description: `Public & <description>`}}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.SEO = store.SEO{Title: "SECRET DRAFT SEO", Description: "SECRET DESCRIPTION", NoIndex: true}
	p, err = app.Service.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.Service.Save(ctx, store.Page{Kind: "post", Slug: "hidden", Title: "Hidden", Document: document, SEO: store.SEO{NoIndex: true}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err = app.Service.Save(ctx, store.Page{Slug: "draft", Title: "DRAFT ONLY", Document: document}, false); err != nil {
		t.Fatal(err)
	}
	public := b.request("GET", "/story?tracking=ignored", "", "", false)
	status(t, public, 200)
	for _, want := range []string{`<title>SEO &#34;title&#34;</title>`, `content="Public &amp; &lt;description&gt;"`, `rel="canonical" href="https://example.com/story"`, `property="og:type" content="article"`, `href="https://example.com/rss.xml"`} {
		if !strings.Contains(public.Body.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(public.Body.String(), "SECRET") {
		t.Fatal("draft metadata leaked")
	}
	preview := b.request("GET", fmt.Sprintf("/admin/pages/%d/preview", p.ID), "", "", false)
	status(t, preview, 200)
	if !strings.Contains(preview.Header().Get("X-Robots-Tag"), "noindex") || !strings.Contains(preview.Body.String(), "SECRET DRAFT SEO") || strings.Contains(preview.Body.String(), `rel="canonical"`) {
		t.Fatal("unsafe preview metadata")
	}
	mapResponse := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, mapResponse, 200)
	var sitemap sitemapSet
	if err = xml.Unmarshal(mapResponse.Body.Bytes(), &sitemap); err != nil {
		t.Fatal(err)
	}
	if sitemap.XMLName.Space != sitemapNamespace || len(sitemap.URLs) != 2 || sitemap.URLs[1].Loc != "https://example.com/story" || sitemap.URLs[1].LastMod == "" {
		t.Fatalf("sitemap %+v", sitemap)
	}
	rssResponse := b.request("GET", "/rss.xml", "", "", false)
	status(t, rssResponse, 200)
	var rss rssFeed
	if err = xml.Unmarshal(rssResponse.Body.Bytes(), &rss); err != nil {
		t.Fatal(err)
	}
	if rss.Version != "2.0" || len(rss.Channel.Items) != 1 || rss.Channel.Title != "Test & Site" || rss.Channel.Items[0].Description != "Public & <description>" || rss.Channel.Items[0].Title != "Story & <news>" {
		t.Fatalf("rss %+v", rss)
	}
	if strings.Contains(rssResponse.Body.String(), "SECRET") {
		t.Fatal("draft RSS leak")
	}
	hidden := b.request("GET", "/hidden", "", "", false)
	status(t, hidden, 200)
	if !strings.Contains(hidden.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatal("missing noindex header")
	}
	missing := b.request("GET", "/missing", "", "", false)
	status(t, missing, 404)
	if !strings.Contains(missing.Header().Get("X-Robots-Tag"), "noindex") || strings.Contains(missing.Body.String(), `rel="canonical"`) {
		t.Fatal("404 indexed")
	}
	robots := b.request("GET", "/robots.txt", "", "", false)
	status(t, robots, 200)
	if !strings.Contains(robots.Body.String(), "Sitemap: https://example.com/sitemap.xml") {
		t.Fatal("robots sitemap missing")
	}
	// Publishing noindex immediately removes the old public URL from discovery.
	if err = s.Publish(ctx, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	var updated sitemapSet
	response := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, response, 200)
	if err = xml.Unmarshal(response.Body.Bytes(), &updated); err != nil || len(updated.URLs) != 1 {
		t.Fatal(updated, err)
	}
}
func TestAppearancePreviewAndActivation(t *testing.T) {
	s, _, b := seoApp(t)
	fields := url.Values{"version": {"0"}, "theme": {"minimal"}, "home_title": {"Unpublished home"}, "home_description": {"A preview description"}, "seo_title": {"Preview SEO"}, "seo_description": {"Preview metadata"}}
	status(t, adminHTML(b, "POST", "/admin/appearance/preview", fields, false, false, false), 403)
	preview := adminHTML(b, "POST", "/admin/appearance/preview", fields, false, false, true)
	status(t, preview, 200)
	if !strings.Contains(preview.Body.String(), `data-theme="minimal"`) || !strings.Contains(preview.Body.String(), "Unpublished home") || !strings.Contains(preview.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("preview missing settings/privacy")
	}
	value, err := s.Appearance()
	if err != nil || value.Version != 0 || value.HomeTitle != "" {
		t.Fatal("preview persisted settings", value, err)
	}
	home := b.request("GET", "/", "", "", false)
	status(t, home, 200)
	if strings.Contains(home.Body.String(), "Unpublished home") {
		t.Fatal("preview leaked")
	}
	page := adminHTML(b, "GET", "/admin/appearance", nil, true, false, false)
	status(t, page, 200)
	if !strings.Contains(page.Body.String(), `data-section="/admin/themes"`) || strings.Contains(page.Body.String(), "<!DOCTYPE") {
		t.Fatal("not SPA appearance")
	}
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 204)
	home = b.request("GET", "/", "", "", false)
	status(t, home, 200)
	if !strings.Contains(home.Body.String(), "Unpublished home") || !strings.Contains(home.Body.String(), "<title>Preview SEO</title>") {
		t.Fatal("settings not applied")
	}
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 409)
	fields.Set("version", "1")
	fields.Set("theme", "../../outside")
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 400)
	fields.Set("theme", "minimal")
	fields.Set("noindex", "1")
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 204)
	emptyMap := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, emptyMap, 200)
	var sitemap sitemapSet
	if err = xml.Unmarshal(emptyMap.Body.Bytes(), &sitemap); err != nil || len(sitemap.URLs) != 0 {
		t.Fatal(sitemap, err)
	}
	// Invalid local templates cannot replace the working built-in theme.
	dir := filepath.Join(s.Dir, "theme")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"layout.html", "page.html", "404.html"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(`{{broken`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fields.Set("version", "2")
	fields.Set("theme", "custom")
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 400)
	value, err = s.Appearance()
	if err != nil || value.Theme != "minimal" || value.Version != 2 {
		t.Fatal(value, err)
	}
	home = b.request("GET", "/", "", "", false)
	status(t, home, 200)
}
func TestSitemapChunksAndConditionalFeed(t *testing.T) {
	s, _, b := seoApp(t)
	_, err := s.DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1001) INSERT INTO pages(slug,version,title,document,kind,published_slug,published_title,published_document,published_at,published_updated_at) SELECT 'p-'||i,1,'Public','{"type":"doc"}','post','p-'||i,'Public','{"type":"doc"}',100,200 FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	indexResponse := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, indexResponse, 200)
	var index sitemapIndex
	if err = xml.Unmarshal(indexResponse.Body.Bytes(), &index); err != nil || len(index.Sitemaps) != 2 {
		t.Fatal(index, err)
	}
	first := b.request("GET", "/sitemap.xml?part=1", "", "", false)
	status(t, first, 200)
	var firstMap sitemapSet
	if err = xml.Unmarshal(first.Body.Bytes(), &firstMap); err != nil || len(firstMap.URLs) != 1000 {
		t.Fatal(len(firstMap.URLs), err)
	}
	second := b.request("GET", "/sitemap.xml?part=2", "", "", false)
	status(t, second, 200)
	var secondMap sitemapSet
	if err = xml.Unmarshal(second.Body.Bytes(), &secondMap); err != nil || len(secondMap.URLs) != 2 {
		t.Fatal(len(secondMap.URLs), err)
	}
	status(t, b.request("GET", "/sitemap.xml?part=3", "", "", false), 404)
	response := b.request("GET", "/rss.xml", "", "", false)
	status(t, response, 200)
	var rss rssFeed
	if err = xml.Unmarshal(response.Body.Bytes(), &rss); err != nil || len(rss.Channel.Items) != 30 {
		t.Fatal(len(rss.Channel.Items), err)
	}
	request := httpRequest(t, b, "/rss.xml", response.Header().Get("ETag"))
	status(t, request, 304)
}

func httpRequest(t *testing.T, b *browser, path, etag string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	b.app.Echo.ServeHTTP(w, r)
	return w
}

func TestEditorSEOFormAutosavesAndRestores(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	original, err := app.Service.Save(ctx, store.Page{Title: "Story", Slug: "story", Document: document, SEO: store.SEO{Title: "Original SEO"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"id": {fmt.Sprint(original.ID)}, "version": {fmt.Sprint(original.Version)}, "title": {"Story"}, "slug": {"story"}, "document": {document}, "kind": {"page"}, "intent": {"autosave"}, "seo_title": {"Draft SEO"}, "seo_description": {"Draft description"}, "noindex": {"1"}}
	response := b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, response, 200)
	draft, err := s.Page(original.ID)
	if err != nil || draft.SEO.Title != "Draft SEO" || draft.SEO.Description != "Draft description" || !draft.SEO.NoIndex {
		t.Fatalf("form lost SEO %+v %v", draft, err)
	}
	status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/revisions/%d/restore", original.ID, original.Version), url.Values{"version": {fmt.Sprint(draft.Version)}}, true, false, true), 303)
	restored, err := s.Page(original.ID)
	if err != nil || restored.SEO != original.SEO {
		t.Fatal(restored, err)
	}
	public, err := s.Public("story")
	if err != nil || public.SEO != original.SEO {
		t.Fatal("restore changed publication", public, err)
	}
}
func TestLocalThemeCanUseSharedSEO(t *testing.T) {
	s, _, b := seoApp(t)
	dir := filepath.Join(s.Dir, "theme")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"layout.html": `{{define "layout"}}<!doctype html><html><head>{{template "seo" .}}</head><body>Local {{.Title}}{{template "body" .}}</body></html>{{end}}`, "page.html": `{{define "body"}}<main>{{.Body}}</main>{{end}}`, "404.html": `{{define "body"}}<h1>Missing</h1>{{end}}`}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	legacy := b.request("GET", "/", "", "", false)
	status(t, legacy, 200)
	if !strings.Contains(legacy.Body.String(), "Local Test &amp; Site") {
		t.Fatal("migration changed legacy theme priority")
	}
	fields := url.Values{"version": {"0"}, "theme": {"custom"}, "seo_title": {"Custom SEO"}}
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 204)
	public := b.request("GET", "/", "", "", false)
	status(t, public, 200)
	if !strings.Contains(public.Body.String(), "<title>Custom SEO</title>") || !strings.Contains(public.Body.String(), `rel="canonical"`) {
		t.Fatal("custom SEO contract missing")
	}
	fields.Set("version", "1")
	fields.Set("theme", "journal")
	status(t, adminHTML(b, "POST", "/admin/appearance", fields, true, false, true), 204)
	public = b.request("GET", "/", "", "", false)
	status(t, public, 200)
	if strings.Contains(public.Body.String(), "Local Test") {
		t.Fatal("built-in theme not activated")
	}
	if _, err := os.Stat(filepath.Join(dir, "layout.html")); err != nil {
		t.Fatal("switch removed local theme", err)
	}
}

func TestThemesAndSEOAreSeparate(t *testing.T) {
	s, _, b := seoApp(t)
	themes := adminHTML(b, "GET", "/admin/themes", nil, false, false, false)
	status(t, themes, 200)
	body := themes.Body.String()
	if strings.Contains(body, `name="seo_title"`) || !strings.Contains(body, `name="theme"`) {
		t.Fatal("theme form contains SEO or misses theme")
	}
	themeLink, module, seoLink := strings.Index(body, `href="/admin/themes"`), strings.Index(body, `Модули</span>`), strings.Index(body, `href="/admin/seo"`)
	if themeLink < 0 || module <= themeLink || seoLink <= module || !strings.Contains(body, `href="/admin/modules"`) {
		t.Fatal("sidebar order/module state")
	}
	seo := adminHTML(b, "GET", "/admin/seo", nil, true, false, false)
	status(t, seo, 200)
	if strings.Contains(seo.Body.String(), `name="theme"`) || !strings.Contains(seo.Body.String(), `name="seo_title"`) {
		t.Fatal("SEO form contains theme or misses metadata")
	}
	status(t, adminHTML(b, "POST", "/admin/seo", url.Values{"version": {"0"}, "seo_title": {"Search title"}, "noindex": {"1"}}, true, false, true), 303)
	a, err := s.Appearance()
	if err != nil || a.Theme != "" || a.SEO.Title != "Search title" || !a.SEO.NoIndex {
		t.Fatal(a, err)
	}
	themeFields := url.Values{"version": {"1"}, "theme": {"minimal"}, "scope": {"theme"}, "home_title": {"Home"}, "seo_title": {"Must be ignored"}}
	status(t, adminHTML(b, "POST", "/admin/themes", themeFields, true, false, true), 204)
	a, err = s.Appearance()
	if err != nil || a.Theme != "minimal" || a.HomeTitle != "Home" || a.SEO.Title != "Search title" || !a.SEO.NoIndex {
		t.Fatal("theme overwrote SEO", a, err)
	}
	status(t, adminHTML(b, "POST", "/admin/seo", url.Values{"version": {"2"}, "seo_title": {"New search title"}, "theme": {"journal"}}, true, false, true), 303)
	a, err = s.Appearance()
	if err != nil || a.Theme != "minimal" || a.HomeTitle != "Home" || a.SEO.Title != "New search title" || a.SEO.NoIndex {
		t.Fatal("SEO overwrote theme", a, err)
	}
	status(t, adminHTML(b, "POST", "/admin/themes", themeFields, true, false, true), 409)
	preview := adminHTML(b, "POST", "/admin/themes/preview", themeFields, false, false, true)
	status(t, preview, 200)
	if !strings.Contains(preview.Body.String(), "New search title") || !strings.Contains(preview.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatal("theme preview lost current SEO/privacy")
	}
	unchanged, err := s.Appearance()
	if err != nil || unchanged.Version != 3 {
		t.Fatal("preview changed settings", unchanged, err)
	}

}
