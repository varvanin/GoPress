package httpui

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestAdvancedSEOSettingsScopesPublicMetadataAndSchema(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	for _, tab := range []string{"search", "index", "social", "schema", "files"} {
		r := b.request("GET", "/admin/seo?tab="+tab, "", "", false)
		status(t, r, 200)
		for _, marker := range []string{`id="seo-panel"`, `hx-target="#seo-panel"`, `hx-select="#seo-panel"`, `hx-swap="outerHTML show:none"`} {
			if !strings.Contains(r.Body.String(), marker) {
				t.Fatal("SEO navigation replaces the whole page", marker)
			}
		}

		if !strings.Contains(r.Body.String(), `name="tab" value="`+tab+`"`) {
			t.Fatal("tab missing", tab)
		}
	}
	save := func(tab string, fields url.Values) {
		a, err := s.Appearance()
		if err != nil {
			t.Fatal(err)
		}
		fields.Set("tab", tab)
		fields.Set("version", strconv.FormatInt(a.Version, 10))
		status(t, adminHTML(b, "POST", "/admin/seo", fields, true, false, true), 303)
	}
	save("search", url.Values{"site_name": {"Engine <brand>"}, "alternate_name": {"Engine"}, "separator": {"—"}, "post_title": {"{title} {separator} {site}"}, "seo_title": {"Home SEO"}, "seo_description": {"Homepage description"}})
	save("social", url.Values{"default_image": {"/uploads/default.webp"}, "twitter_site": {"@gopress"}, "twitter_card": {"summary_large_image"}, "og_locale": {"ru_RU"}, "social_title": {"Social home"}})
	save("schema", url.Values{"schema_enabled": {"1"}, "organization_type": {"Organization"}, "organization_name": {"Company </script><script>bad</script>"}, "logo_url": {"https://example.com/logo.png"}, "social_profiles": {"https://example.org/profile"}, "google_verification": {"token<&"}})
	p, err := app.Service.Save(ctx, store.Page{Title: "Article", Slug: "article", Kind: "post", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	public := b.request("GET", "/article", "", "", false)
	status(t, public, 200)
	for _, want := range []string{`<title>Article — Engine &lt;brand&gt;</title>`, `name="twitter:site" content="@gopress"`, `property="og:image" content="https://example.com/uploads/default.webp"`, `name="google-site-verification" content="token&lt;&amp;"`, `BlogPosting`, `BreadcrumbList`} {
		if !strings.Contains(public.Body.String(), want) {
			t.Fatal("missing", want)
		}
	}
	home := b.request("GET", "/", "", "", false)
	status(t, home, 200)
	blocks := regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(home.Body.String())
	if len(blocks) != 2 {
		t.Fatal("missing JSON-LD", home.Body.String())
	}
	var data map[string]any
	if err = json.Unmarshal([]byte(blocks[1]), &data); err != nil {
		t.Fatal("invalid JSON-LD", err)
	}
	if strings.Contains(blocks[1], "</script>") {
		t.Fatal("script escaped incorrectly")
	}
	p.SEO = store.SEO{Title: "Individual SEO", SocialTitle: "Individual card", SocialImage: "https://example.org/individual.webp", Canonical: "https://example.org/source", NoFollow: true}
	p, err = app.Service.Save(ctx, p, true)
	if err != nil {
		t.Fatal(err)
	}
	individual := b.request("GET", "/article", "", "", false)
	status(t, individual, 200)
	for _, want := range []string{`<title>Individual SEO</title>`, `property="og:title" content="Individual card"`, `rel="canonical" href="https://example.org/source"`, `content="index, nofollow"`} {
		if !strings.Contains(individual.Body.String(), want) {
			t.Fatal("individual override missing", want)
		}
	}
	p.SEO.SocialTitle = "SECRET DRAFT SOCIAL"
	if _, err = app.Service.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.request("GET", "/article", "", "", false).Body.String(), "SECRET") {
		t.Fatal("draft social metadata leaked")
	}
	// Switching templates and saving another SEO tab must preserve advanced settings.
	a, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	a.Theme = "minimal"
	if err = app.Service.SaveAppearance(ctx, a); err != nil {
		t.Fatal(err)
	}
	save("index", url.Values{"noindex_posts": {"1"}, "max_snippet": {"120"}, "max_image_preview": {"large"}})
	after, err := s.Appearance()
	if err != nil || after.SEOOptions.SiteName != "Engine <brand>" || after.SEOOptions.TwitterSite != "@gopress" || !after.SEOOptions.SchemaEnabled {
		t.Fatal("settings lost", after, err)
	}
	hidden := b.request("GET", "/article", "", "", false)
	status(t, hidden, 200)
	if !strings.Contains(hidden.Header().Get("X-Robots-Tag"), "noindex") || !strings.Contains(hidden.Body.String(), "max-snippet:120") || strings.Contains(hidden.Body.String(), "application/ld+json") {
		t.Fatal("robots settings ignored")
	}
	legacy := url.Values{"version": {fmt.Sprint(after.Version)}, "seo_title": {"Legacy save"}}
	status(t, adminHTML(b, "POST", "/admin/seo", legacy, false, false, true), 303)
	after, _ = s.Appearance()
	if after.SEOOptions.SiteName != "Engine <brand>" {
		t.Fatal("legacy save wiped options")
	}
}
func TestAdvancedSEOFilesFiltersAndValidation(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	for _, p := range []store.Page{{Title: "Page", Slug: "about", Document: document}, {Title: "Post", Slug: "post", Kind: "post", Document: document}, {Title: "Self", Slug: "self", Document: document, SEO: store.SEO{Canonical: "https://example.com/self"}}, {Title: "Duplicate", Slug: "duplicate", Document: document, SEO: store.SEO{Canonical: "https://example.org/original"}}} {
		if _, err := app.Service.Save(ctx, p, true); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := s.Appearance()
	a.SEOOptions.ExcludeSitemapPaths = "/about"
	a.SEOOptions.ExcludeSitemapPosts = true
	if err := app.Service.SaveSEOSettings(ctx, a.Version, a.SEO, a.SEOOptions); err != nil {
		t.Fatal(err)
	}
	r := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, r, 200)
	var sitemap sitemapSet
	if err := xml.Unmarshal(r.Body.Bytes(), &sitemap); err != nil {
		t.Fatal(err)
	}
	if len(sitemap.URLs) != 2 || sitemap.URLs[1].Loc != "https://example.com/self" {
		t.Fatal("sitemap filters", sitemap)
	}
	a, _ = s.Appearance()
	a.SEOOptions.RobotsText = "User-agent: *\nDisallow: /private\nSitemap: {site_url}/sitemap.xml"
	a.SEOOptions.DisableRSS = true
	if err := app.Service.SaveSEOSettings(ctx, a.Version, a.SEO, a.SEOOptions); err != nil {
		t.Fatal(err)
	}
	robots := b.request("GET", "/robots.txt", "", "", false)
	status(t, robots, 200)
	if !strings.Contains(robots.Body.String(), "Disallow: /private") || !strings.Contains(robots.Body.String(), "https://example.com/sitemap.xml") {
		t.Fatal(robots.Body.String())
	}
	status(t, b.request("GET", "/rss.xml", "", "", false), 404)
	for _, id := range []string{"journal", "minimal", "studio"} {
		appearance, err := s.Appearance()
		if err != nil {
			t.Fatal(err)
		}
		appearance.Theme = id
		if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
			t.Fatal(err)
		}
		home := b.request("GET", "/", "", "", false)
		status(t, home, 200)
		if strings.Contains(home.Body.String(), `href="/rss.xml"`) || strings.Contains(home.Body.String(), `application/rss+xml`) {
			t.Fatal("disabled RSS still linked", id)
		}
	}

	a, _ = s.Appearance()
	a.SEOOptions.DisableSitemap = true
	if err := app.Service.SaveSEOSettings(ctx, a.Version, a.SEO, a.SEOOptions); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", "/sitemap.xml", "", "", false), 404)
	invalid := url.Values{"tab": {"social"}, "version": {fmt.Sprint(a.Version + 1)}, "default_image": {"javascript:alert(1)"}, "social_title": {"Keep my invalid submission"}}
	status(t, adminHTML(b, "POST", "/admin/seo", invalid, false, false, false), 403)
	bad := adminHTML(b, "POST", "/admin/seo", invalid, true, false, true)
	status(t, bad, 400)
	if !strings.Contains(bad.Body.String(), "Keep my invalid submission") {
		t.Fatal("invalid submission lost")
	}
	stale := url.Values{"tab": {"search"}, "version": {"0"}, "seo_title": {"Unsaved conflict title"}}
	conflict := adminHTML(b, "POST", "/admin/seo", stale, true, false, true)
	status(t, conflict, 409)
	if !strings.Contains(conflict.Body.String(), "Unsaved conflict title") {
		t.Fatal("conflict lost edits")
	}
}

func TestSitemapOmitsUnavailableSelectedHome(t *testing.T) {
	s, _, b := seoApp(t)
	// Simulate a previously selected home page that is no longer available.
	_, err := s.DB.Exec(`UPDATE settings SET value=json_set(value,'$.home_mode','page','$.home_page_id',12345)`)
	if err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/sitemap.xml", "", "", false)
	status(t, response, 200)
	var result sitemapSet
	if err = xml.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.URLs) != 0 {
		t.Fatal(result, err)
	}
}
