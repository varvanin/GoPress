package service

import (
	"gopress/internal/store"
	"testing"
)

func TestAdvancedSEOValidation(t *testing.T) {
	for _, o := range []store.SEOOptions{{PageTitle: "{unknown}"}, {PostTitle: "{title"}, {DefaultImage: "javascript:alert(1)"}, {LogoURL: "//evil.test/logo"}, {MaxSnippet: "-2"}, {MaxVideoPreview: "infinite"}, {MaxImagePreview: "gigantic"}, {Locale: "bad locale"}, {TwitterSite: "missing-at"}, {SocialProfiles: "https://ok.test\njavascript:bad"}, {ExcludeSitemapPaths: "https://outside.test"}, {RobotsText: "bad\x00text"}} {
		if CleanSEOOptions(&o) == nil {
			t.Fatal("invalid options accepted", o)
		}
	}
	valid := store.SEOOptions{PageTitle: "{title} {separator} {site}", DefaultImage: "/uploads/image.webp", MaxSnippet: "-1", MaxImagePreview: "large", Locale: "pl_PL", TwitterSite: "@example", SocialProfiles: "https://example.org/profile", ExcludeSitemapPaths: "/\n/about"}
	if err := CleanSEOOptions(&valid); err != nil {
		t.Fatal(err)
	}
	for _, seo := range []store.SEO{{Canonical: "/relative"}, {Canonical: "javascript:bad"}, {SocialImage: "//outside.test"}} {
		if cleanSEO(&seo) == nil {
			t.Fatal("bad individual SEO", seo)
		}
	}
}
