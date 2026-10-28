package store

// Zero values preserve the behavior of sites configured before advanced SEO.
type SEOOptions struct {
	SiteName, AlternateName, Separator                                                             string
	PageTitle, PostTitle, CategoryTitle, SearchTitle, DescriptionFallback                          string
	NoIndexHome, NoIndexPages, NoIndexPosts, NoIndexCategories, NoIndexPagination                  bool
	NoFollow, NoSnippet, NoImageIndex, DisableCanonical                                            bool
	MaxSnippet, MaxVideoPreview, MaxImagePreview                                                   string
	DisableOpenGraph, DisableTwitter                                                               bool
	DefaultImage, SocialTitle, SocialDescription, Locale, TwitterSite, TwitterCard                 string
	SchemaEnabled                                                                                  bool
	OrganizationType, OrganizationName, LogoURL, SocialProfiles                                    string
	GoogleVerification, BingVerification, YandexVerification                                       string
	DisableSitemap, ExcludeSitemapPages, ExcludeSitemapPosts, IncludeSitemapCategories, DisableRSS bool
	ExcludeSitemapPaths, RobotsText                                                                string
}
