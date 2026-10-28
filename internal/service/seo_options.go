package service

import (
	"context"
	"gopress/internal/store"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func validSEOURL(value string, relative bool) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	if relative && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return u.Host == "" && u.Scheme == ""
	}
	return (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
func cleanIndividualSEO(seo *store.SEO) error {
	seo.Canonical = strings.TrimSpace(seo.Canonical)
	seo.SocialImage = strings.TrimSpace(seo.SocialImage)
	seo.SocialTitle = strings.Join(strings.Fields(seo.SocialTitle), " ")
	seo.SocialDescription = strings.Join(strings.Fields(seo.SocialDescription), " ")
	if !utf8.ValidString(seo.Canonical+seo.SocialImage+seo.SocialTitle+seo.SocialDescription) || len(seo.Canonical) > 2048 || len(seo.SocialImage) > 2048 || utf8.RuneCountInString(seo.SocialTitle) > 200 || utf8.RuneCountInString(seo.SocialDescription) > 500 || !validSEOURL(seo.Canonical, false) || !validSEOURL(seo.SocialImage, true) {
		return ErrInvalid
	}
	return nil
}

var seoPlaceholder = regexp.MustCompile(`\{[^{}]+\}`)

func CleanSEOOptions(o *store.SEOOptions) error {
	short := []*string{&o.SiteName, &o.AlternateName, &o.Separator, &o.OrganizationName, &o.Locale, &o.TwitterSite, &o.GoogleVerification, &o.BingVerification, &o.YandexVerification, &o.SocialTitle}
	for _, v := range short {
		*v = strings.Join(strings.Fields(*v), " ")
		if !utf8.ValidString(*v) || utf8.RuneCountInString(*v) > 200 {
			return ErrInvalid
		}
	}
	if utf8.RuneCountInString(o.Separator) > 5 {
		return ErrInvalid
	}
	for _, pattern := range []*string{&o.PageTitle, &o.PostTitle, &o.CategoryTitle, &o.SearchTitle} {
		*pattern = strings.TrimSpace(*pattern)
		if !utf8.ValidString(*pattern) || utf8.RuneCountInString(*pattern) > 300 {
			return ErrInvalid
		}
		remaining := seoPlaceholder.ReplaceAllStringFunc(*pattern, func(key string) string {
			switch key {
			case "{title}", "{site}", "{separator}", "{page}", "{category}", "{query}":
				return ""
			}
			return key
		})
		if strings.ContainsAny(remaining, "{}") || strings.ContainsAny(*pattern, "\r\n") {
			return ErrInvalid
		}
	}
	for _, v := range []*string{&o.DescriptionFallback, &o.SocialDescription} {
		*v = strings.Join(strings.Fields(*v), " ")
		if !utf8.ValidString(*v) || utf8.RuneCountInString(*v) > 500 {
			return ErrInvalid
		}
	}
	for _, v := range []*string{&o.DefaultImage, &o.LogoURL} {
		*v = strings.TrimSpace(*v)
		if len(*v) > 2048 || !validSEOURL(*v, true) {
			return ErrInvalid
		}
	}
	for _, v := range []string{o.MaxSnippet, o.MaxVideoPreview} {
		if v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < -1 || n > 100000 {
				return ErrInvalid
			}
		}
	}
	switch o.MaxImagePreview {
	case "", "none", "standard", "large":
	default:
		return ErrInvalid
	}
	switch o.TwitterCard {
	case "", "summary", "summary_large_image":
	default:
		return ErrInvalid
	}
	switch o.OrganizationType {
	case "", "Organization", "Person":
	default:
		return ErrInvalid
	}
	if o.Locale != "" && !regexp.MustCompile(`^[a-z]{2,3}(?:_[A-Z]{2})?$`).MatchString(o.Locale) {
		return ErrInvalid
	}
	if o.TwitterSite != "" && !regexp.MustCompile(`^@[A-Za-z0-9_]{1,15}$`).MatchString(o.TwitterSite) {
		return ErrInvalid
	}
	o.SocialProfiles = strings.TrimSpace(strings.ReplaceAll(o.SocialProfiles, "\r\n", "\n"))
	profiles := strings.Split(o.SocialProfiles, "\n")
	if len(profiles) > 20 {
		return ErrInvalid
	}
	for _, v := range profiles {
		if !validSEOURL(strings.TrimSpace(v), false) || len(v) > 2048 {
			return ErrInvalid
		}
	}
	o.ExcludeSitemapPaths = strings.TrimSpace(strings.ReplaceAll(o.ExcludeSitemapPaths, "\r\n", "\n"))
	paths := strings.Split(o.ExcludeSitemapPaths, "\n")
	if len(paths) > 100 {
		return ErrInvalid
	}
	for _, v := range paths {
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || u.RawQuery != "" || u.Fragment != "" || len(v) > 512 {
			return ErrInvalid
		}
	}
	o.RobotsText = strings.TrimSpace(strings.ReplaceAll(o.RobotsText, "\r\n", "\n"))
	if len(o.RobotsText) > 16384 || !utf8.ValidString(o.RobotsText) || strings.ContainsAny(o.RobotsText, "\x00\r") {
		return ErrInvalid
	}
	return nil
}
func (v *Service) SaveSEOSettings(ctx context.Context, version int64, seo store.SEO, options store.SEOOptions) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	if version < 0 {
		return ErrInvalid
	}
	if err := cleanSEO(&seo); err != nil {
		return err
	}
	if err := CleanSEOOptions(&options); err != nil {
		return err
	}
	a, err := v.Store.Appearance()
	if err != nil {
		return err
	}
	a.Version = version
	a.SEO = seo
	a.SEOOptions = options
	return v.Store.SaveAppearance(ctx, a)
}
