package httpui

import (
	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
	"strconv"
)

func seoTab(c *echo.Context) string {
	tab := c.QueryParam("tab")
	if c.Request().Method == "POST" {
		tab = c.FormValue("tab")
	}
	switch tab {
	case "index", "social", "schema", "files":
		return tab
	}
	return "search"
}
func (a *App) postedSEOSettings(c *echo.Context) (store.Appearance, error) {
	aValue, err := a.Service.Store.Appearance()
	if err != nil {
		return aValue, err
	}
	aValue.Version, err = strconv.ParseInt(c.FormValue("version"), 10, 64)
	if err != nil {
		return aValue, service.ErrInvalid
	}
	o := &aValue.SEOOptions
	check := func(name string) bool { return c.FormValue(name) == "1" }
	get := c.FormValue
	switch seoTab(c) {
	case "search":
		aValue.SEO.Title = get("seo_title")
		aValue.SEO.Description = get("seo_description")
		if c.FormValue("tab") != "" {
			o.SiteName = get("site_name")
			o.AlternateName = get("alternate_name")
			o.Separator = get("separator")
			o.PageTitle = get("page_title")
			o.PostTitle = get("post_title")
			o.CategoryTitle = get("category_title")
			o.SearchTitle = get("search_title")
			o.DescriptionFallback = get("description_fallback")
		}
		// Older clients submit the original three-field form without a tab.
		if c.FormValue("tab") == "" {
			aValue.SEO.NoIndex = check("noindex")
		}
	case "index":
		o.NoIndexHome = check("noindex_home")
		aValue.SEO.NoIndex = check("noindex")
		o.NoIndexPages = check("noindex_pages")
		o.NoIndexPosts = check("noindex_posts")
		o.NoIndexCategories = check("noindex_categories")
		o.NoIndexPagination = check("noindex_pagination")
		o.NoFollow = check("nofollow")
		o.NoSnippet = check("nosnippet")
		o.NoImageIndex = check("noimageindex")
		o.DisableCanonical = check("disable_canonical")
		o.MaxSnippet = get("max_snippet")
		o.MaxVideoPreview = get("max_video_preview")
		o.MaxImagePreview = get("max_image_preview")
	case "social":
		o.DisableOpenGraph = check("disable_og")
		o.DisableTwitter = check("disable_twitter")
		o.DefaultImage = get("default_image")
		o.SocialTitle = get("social_title")
		o.SocialDescription = get("social_description")
		o.Locale = get("og_locale")
		o.TwitterSite = get("twitter_site")
		o.TwitterCard = get("twitter_card")
	case "schema":
		o.SchemaEnabled = check("schema_enabled")
		o.OrganizationType = get("organization_type")
		o.OrganizationName = get("organization_name")
		o.LogoURL = get("logo_url")
		o.SocialProfiles = get("social_profiles")
		o.GoogleVerification = get("google_verification")
		o.BingVerification = get("bing_verification")
		o.YandexVerification = get("yandex_verification")
	case "files":
		o.DisableSitemap = check("disable_sitemap")
		o.ExcludeSitemapPages = check("sitemap_exclude_pages")
		o.ExcludeSitemapPosts = check("sitemap_exclude_posts")
		o.IncludeSitemapCategories = check("sitemap_categories")
		o.DisableRSS = check("disable_rss")
		o.ExcludeSitemapPaths = get("sitemap_exclude_paths")
		o.RobotsText = get("robots_text")
	}
	return aValue, nil
}
func seoPreviewTitle(site store.Site, a store.Appearance) string {
	if a.SEO.Title != "" {
		return a.SEO.Title
	}
	if a.SEOOptions.SiteName != "" {
		return a.SEOOptions.SiteName
	}
	return site.Title
}
func seoPreviewDescription(a store.Appearance) string {
	if a.SEO.Description != "" {
		return a.SEO.Description
	}
	if a.SEOOptions.DescriptionFallback != "" {
		return a.SEOOptions.DescriptionFallback
	}
	return a.HomeDescription
}
