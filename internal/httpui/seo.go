package httpui

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

type PageMeta struct {
	Title, Description, Canonical, Image, Type                                         string
	NoIndex                                                                            bool
	Robots, SocialTitle, SocialDescription, SiteName, Locale, TwitterSite, TwitterCard string
	GoogleVerification, BingVerification, YandexVerification                           string
	OpenGraph, Twitter, RSS                                                            bool
	JSONLD                                                                             template.JS
}

func pageMetadata(site store.Site, a store.Appearance, p store.Page, home, private, notFound bool, page int, term int64) PageMeta {
	m := PageMeta{Title: p.Title + " · " + site.Title, Description: p.SEO.Description, Type: "website", NoIndex: a.SEO.NoIndex || p.SEO.NoIndex || private || notFound}
	if p.SEO.Title != "" {
		m.Title = p.SEO.Title
	}
	if p.Kind == "post" {
		m.Type = "article"
	}
	if home {
		m.Title = site.Title
		if a.SEO.Title != "" {
			m.Title = a.SEO.Title
		}
		m.Description = a.SEO.Description
		if m.Description == "" {
			m.Description = a.HomeDescription
		}
		if m.Description == "" {
			m.Description = "Последние записи — " + site.Title
		}
	} else if m.Description == "" {
		m.Description = service.Excerpt(p.Document, 160)
	}
	if !private && !notFound {
		m.Canonical = site.BaseURL + "/" + p.Slug
		if home {
			m.Canonical = site.BaseURL + "/"
			q := url.Values{}
			if page > 1 {
				q.Set("page", strconv.Itoa(page))
			}
			if term > 0 {
				q.Set("term", strconv.FormatInt(term, 10))
			}
			if len(q) > 0 {
				m.Canonical += "?" + q.Encode()
			}
		}
	}
	if p.Cover != "" {
		m.Image = site.BaseURL + "/uploads/" + p.Cover
	}
	return m
}
func xmlResponse(c *echo.Context, mime string, value any) error {
	body, err := xml.Marshal(value)
	if err != nil {
		return err
	}
	body = append([]byte(xml.Header), body...)
	digest := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	c.Response().Header().Set("ETag", etag)
	c.Response().Header().Set("Cache-Control", "no-cache")
	if c.Request().Header.Get("If-None-Match") == etag {
		return c.NoContent(http.StatusNotModified)
	}
	return c.Blob(200, mime+"; charset=utf-8", body)
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}
type sitemapSet struct {
	XMLName   xml.Name     `xml:"urlset"`
	Namespace string       `xml:"xmlns,attr"`
	URLs      []sitemapURL `xml:"url"`
}
type sitemapIndex struct {
	XMLName   xml.Name     `xml:"sitemapindex"`
	Namespace string       `xml:"xmlns,attr"`
	Sitemaps  []sitemapURL `xml:"sitemap"`
}

const sitemapNamespace = "http://www.sitemaps.org/schemas/sitemap/0.9"
const sitemapPageSize = 1000

func (a *App) sitemap(c *echo.Context) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	o := appearance.SEOOptions
	if o.DisableSitemap {
		return echo.NewHTTPError(404, "Sitemap disabled")
	}
	total := 0
	extras := []sitemapURL{}
	if !appearance.SEO.NoIndex {
		includeHome := !o.NoIndexHome
		for _, line := range strings.Split(o.ExcludeSitemapPaths, "\n") {
			if strings.TrimSpace(line) == "/" {
				includeHome = false
			}
		}
		settings, err := a.Service.Store.Settings()
		if err != nil {
			return err
		}
		if settings.HomeMode == "page" {
			home, err := a.Service.Store.PublicPage(settings.HomePageID)
			if errors.Is(err, sql.ErrNoRows) {
				includeHome = false
			} else if err != nil {
				return err
			} else {
				o.ExcludeSitemapPaths += "\n/" + home.Slug
				if home.SEO.NoIndex || home.SEO.Canonical != "" && home.SEO.Canonical != site.BaseURL+"/" {
					includeHome = false
				}
			}
		}
		if includeHome {
			extras = append(extras, sitemapURL{Loc: site.BaseURL + "/"})
		}
		if o.IncludeSitemapCategories && !o.NoIndexCategories && settings.HomeMode != "page" {
			terms, err := a.Service.Store.Terms(true)
			if err != nil {
				return err
			}
			for _, term := range terms {
				if term.Kind == "category" {
					extras = append(extras, sitemapURL{Loc: canonicalCategoryURL(site, term.ID)})
				}
			}
		}
	} else {
		total = 0
	}
	if !appearance.SEO.NoIndex {
		total, err = a.Service.Store.SitemapCount(o)
		if err != nil {
			return err
		}
	}
	total += len(extras)
	chunks := max(1, (total+sitemapPageSize-1)/sitemapPageSize)
	part := 0
	if raw := c.QueryParam("part"); raw != "" {
		part, err = strconv.Atoi(raw)
		if err != nil || part < 1 || part > chunks {
			return echo.NewHTTPError(404, "Not found")
		}
	}
	if chunks > 1 && part == 0 {
		index := sitemapIndex{Namespace: sitemapNamespace}
		for i := 1; i <= chunks; i++ {
			index.Sitemaps = append(index.Sitemaps, sitemapURL{Loc: site.BaseURL + "/sitemap.xml?part=" + strconv.Itoa(i)})
		}
		return xmlResponse(c, "application/xml", index)
	}
	if part == 0 {
		part = 1
	}
	set := sitemapSet{Namespace: sitemapNamespace}
	offset := (part - 1) * sitemapPageSize
	if !appearance.SEO.NoIndex {
		for i := offset; i < min(len(extras), offset+sitemapPageSize); i++ {
			set.URLs = append(set.URLs, extras[i])
		}
		limit := sitemapPageSize - len(set.URLs)
		if limit > 0 {
			pages, err := a.Service.Store.SitemapPages(max(0, offset-len(extras)), limit, o)
			if err != nil {
				return err
			}
			for _, p := range pages {
				entry := sitemapURL{Loc: site.BaseURL + "/" + p.Slug}
				if p.PublishedUpdatedAt > 0 {
					entry.LastMod = time.Unix(p.PublishedUpdatedAt, 0).UTC().Format(time.RFC3339)
				}
				set.URLs = append(set.URLs, entry)
			}
		}
	}
	return xmlResponse(c, "application/xml", set)
}

type rssGUID struct {
	PermaLink string `xml:"isPermaLink,attr"`
	Value     string `xml:",chardata"`
}
type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	Description string  `xml:"description"`
	GUID        rssGUID `xml:"guid"`
	Date        string  `xml:"pubDate"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

func (a *App) rss(c *echo.Context) error {
	settings, err := a.Service.Store.Settings()
	if err != nil {
		return err
	}
	site := settings.Site
	appearance, err := a.Service.Store.Appearance()
	if err != nil {
		return err
	}
	if appearance.SEOOptions.DisableRSS {
		return echo.NewHTTPError(404, "RSS disabled")
	}
	feed := rssFeed{Version: "2.0", Channel: rssChannel{Title: site.Title, Link: site.BaseURL + "/", Description: appearance.SEO.Description}}
	if feed.Channel.Description == "" {
		feed.Channel.Description = settings.Description
	}
	if feed.Channel.Description == "" {
		feed.Channel.Description = "Последние записи — " + site.Title
	}
	if !appearance.SEO.NoIndex {
		posts, e := a.Service.Store.FeedPosts()
		if e != nil {
			return e
		}
		for _, p := range posts {
			description := p.SEO.Description
			if description == "" {
				description = service.Excerpt(p.Document, 500)
			}
			feed.Channel.Items = append(feed.Channel.Items, rssItem{Title: p.Title, Link: site.BaseURL + "/" + p.Slug, Description: description, GUID: rssGUID{PermaLink: "false", Value: site.BaseURL + "/posts/" + strconv.FormatInt(p.ID, 10)}, Date: time.Unix(p.PublishedAt, 0).UTC().Format(time.RFC1123Z)})
		}
	}
	return xmlResponse(c, "application/rss+xml", feed)
}
func (a *App) seoRoutes() {
	a.Echo.GET("/sitemap.xml", a.sitemap)
	a.Echo.GET("/rss.xml", a.rss)
	a.Echo.GET("/robots.txt", func(c *echo.Context) error {
		site, err := a.Service.Store.Site()
		if err != nil {
			return err
		}
		appearance, err := a.Service.Store.Appearance()
		if err != nil {
			return err
		}
		return c.String(200, robotsContent(site, appearance))
	})
}
