package httpui

import (
	"encoding/json"
	"gopress/internal/store"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func seoPattern(pattern, fallback, title, site, separator, category, query string, page int) string {
	if pattern == "" {
		pattern = fallback
	}
	return strings.NewReplacer("{title}", title, "{site}", site, "{separator}", separator, "{category}", category, "{query}", query, "{page}", strconv.Itoa(page)).Replace(pattern)
}
func absoluteSEOURL(base, value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return strings.TrimSuffix(base, "/") + value
	}
	return value
}
func applySEOOptions(m *PageMeta, site store.Site, a store.Appearance, p store.Page, home, private, missing bool, page int, term int64, terms []store.Term, search *PublicSearchData, pages []store.Page) {
	o := a.SEOOptions
	siteName := o.SiteName
	if siteName == "" {
		siteName = site.Title
	}
	sep := o.Separator
	if sep == "" {
		sep = "·"
	}
	category := ""
	for _, t := range terms {
		if t.ID == term {
			category = t.Name
			break
		}
	}
	if p.SEO.Title == "" && !home && !missing && search == nil {
		pattern := o.PageTitle
		if p.Kind == "post" {
			pattern = o.PostTitle
		}
		m.Title = seoPattern(pattern, "{title} {separator} {site}", p.Title, siteName, sep, category, "", page)
	}
	if home && term > 0 {
		m.Title = seoPattern(o.CategoryTitle, "{category} {separator} {site}", p.Title, siteName, sep, category, "", page)
	}
	if home && page > 1 && (term == 0 || !strings.Contains(o.CategoryTitle, "{page}")) {
		m.Title += " " + sep + " " + strconv.Itoa(page)
	}
	if search != nil {
		m.Title = seoPattern(o.SearchTitle, "{query} {separator} {site}", p.Title, siteName, sep, "", search.Query, search.Page)
		m.NoIndex = true
		if !private && !missing {
			q := url.Values{"q": {search.Query}}
			if search.Page > 1 {
				q.Set("page", strconv.Itoa(search.Page))
			}
			m.Canonical = site.BaseURL + "/search?" + q.Encode()
		}
	}
	if home && term == 0 && a.SEO.Title == "" && page == 1 {
		m.Title = siteName
	}
	if o.DescriptionFallback != "" && p.SEO.Description == "" && (!home || a.SEO.Description == "") {
		m.Description = o.DescriptionFallback
	}
	m.NoIndex = m.NoIndex || home && term == 0 && page == 1 && o.NoIndexHome || p.Kind == "page" && !home && o.NoIndexPages || p.Kind == "post" && o.NoIndexPosts || term > 0 && o.NoIndexCategories || home && page > 1 && o.NoIndexPagination
	robots := []string{"index", "follow"}
	if m.NoIndex {
		robots[0] = "noindex"
	}
	if private || missing || o.NoFollow || p.SEO.NoFollow {
		robots[1] = "nofollow"
	}
	if o.NoSnippet {
		robots = append(robots, "nosnippet")
	}
	if o.NoImageIndex {
		robots = append(robots, "noimageindex")
	}
	if o.MaxSnippet != "" {
		robots = append(robots, "max-snippet:"+o.MaxSnippet)
	}
	if o.MaxVideoPreview != "" {
		robots = append(robots, "max-video-preview:"+o.MaxVideoPreview)
	}
	if o.MaxImagePreview != "" {
		robots = append(robots, "max-image-preview:"+o.MaxImagePreview)
	}
	m.Robots = strings.Join(robots, ", ")
	if !private && !missing && p.SEO.Canonical != "" {
		m.Canonical = p.SEO.Canonical
	}
	if o.DisableCanonical {
		m.Canonical = ""
	}
	m.OpenGraph = !o.DisableOpenGraph
	m.Twitter = !o.DisableTwitter
	m.RSS = !o.DisableRSS
	m.SiteName = siteName
	m.Locale = o.Locale
	m.TwitterSite = o.TwitterSite
	m.TwitterCard = o.TwitterCard
	m.SocialTitle = p.SEO.SocialTitle
	if m.SocialTitle == "" && home {
		m.SocialTitle = o.SocialTitle
	}
	if m.SocialTitle == "" {
		m.SocialTitle = m.Title
	}
	m.SocialDescription = p.SEO.SocialDescription
	if m.SocialDescription == "" && home {
		m.SocialDescription = o.SocialDescription
	}
	if m.SocialDescription == "" {
		m.SocialDescription = m.Description
	}
	if p.SEO.SocialImage != "" {
		m.Image = absoluteSEOURL(site.BaseURL, p.SEO.SocialImage)
	} else if m.Image == "" {
		m.Image = absoluteSEOURL(site.BaseURL, o.DefaultImage)
	}
	if m.TwitterCard == "" {
		m.TwitterCard = "summary"
		if m.Image != "" {
			m.TwitterCard = "summary_large_image"
		}
	}
	m.GoogleVerification = o.GoogleVerification
	m.BingVerification = o.BingVerification
	m.YandexVerification = o.YandexVerification
	if o.SchemaEnabled && !private && !missing && !m.NoIndex {
		graph := []any{}
		if home && page == 1 && term == 0 {
			website := map[string]any{"@type": "WebSite", "@id": site.BaseURL + "/#website", "url": site.BaseURL + "/", "name": siteName}
			if o.AlternateName != "" {
				website["alternateName"] = o.AlternateName
			}
			graph = append(graph, website)
			if o.OrganizationType != "" && o.OrganizationName != "" {
				entity := map[string]any{"@type": o.OrganizationType, "@id": site.BaseURL + "/#publisher", "name": o.OrganizationName, "url": site.BaseURL + "/"}
				if o.LogoURL != "" {
					if o.OrganizationType == "Person" {
						entity["image"] = absoluteSEOURL(site.BaseURL, o.LogoURL)
					} else {
						entity["logo"] = absoluteSEOURL(site.BaseURL, o.LogoURL)
					}
				}
				profiles := []string{}
				for _, line := range strings.Split(o.SocialProfiles, "\n") {
					if v := strings.TrimSpace(line); v != "" {
						profiles = append(profiles, v)
					}
				}
				if len(profiles) > 0 {
					entity["sameAs"] = profiles
				}
				graph = append(graph, entity)
			}
		}
		if !home && p.Kind == "post" {
			article := map[string]any{"@type": "BlogPosting", "headline": p.Title, "description": m.Description, "url": absoluteSEOURL(site.BaseURL, "/"+p.Slug)}
			if m.Image != "" {
				article["image"] = m.Image
			}
			if p.PublishedAt > 0 {
				article["datePublished"] = time.Unix(p.PublishedAt, 0).UTC().Format(time.RFC3339)
			}
			if p.PublishedUpdatedAt > 0 {
				article["dateModified"] = time.Unix(p.PublishedUpdatedAt, 0).UTC().Format(time.RFC3339)
			}
			graph = append(graph, article)
		}
		if !home && p.Slug != "" && search == nil {
			items := []any{map[string]any{"@type": "ListItem", "position": 1, "name": siteName, "item": site.BaseURL + "/"}}
			parts := strings.Split(p.Slug, "/")
			for i := 1; i < len(parts); i++ {
				slug := strings.Join(parts[:i], "/")
				for _, parent := range pages {
					if parent.Slug == slug {
						items = append(items, map[string]any{"@type": "ListItem", "position": len(items) + 1, "name": parent.Title, "item": site.BaseURL + "/" + slug})
						break
					}
				}
			}
			items = append(items, map[string]any{"@type": "ListItem", "position": len(items) + 1, "name": p.Title, "item": site.BaseURL + "/" + p.Slug})
			graph = append(graph, map[string]any{"@type": "BreadcrumbList", "itemListElement": items})
		}
		if len(graph) > 0 {
			raw, _ := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": graph})
			m.JSONLD = template.JS(raw)
		}
	}
}
func robotsContent(site store.Site, a store.Appearance) string {
	if a.SEOOptions.RobotsText != "" {
		return strings.ReplaceAll(a.SEOOptions.RobotsText, "{site_url}", site.BaseURL) + "\n"
	}
	lines := []string{"User-agent: *", "Disallow: /admin", "Disallow: /login", "Disallow: /install", "Disallow: /forgot-password", "Disallow: /reset-password"}
	if !a.SEOOptions.DisableSitemap {
		lines = append(lines, "Sitemap: "+site.BaseURL+"/sitemap.xml")
	}
	return strings.Join(lines, "\n") + "\n"
}
func canonicalCategoryURL(site store.Site, id int64) string {
	return site.BaseURL + "/?" + url.Values{"term": {strconv.FormatInt(id, 10)}}.Encode()
}
