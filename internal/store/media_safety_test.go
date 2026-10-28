package store

import (
	"context"
	"errors"
	"testing"
)

func TestMediaDetailsRejectStaleAndMissingRevision(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Media("photo.png", "image/png", 123, 1); err != nil {
		t.Fatal(err)
	}
	first, err := s.MediaFile("photo.png")
	if err != nil {
		t.Fatal(err)
	}
	stale := first
	first.Title = "First editor"
	if err = s.SaveMediaDetails(first); err != nil {
		t.Fatal(err)
	}
	stale.Title = "Second editor"
	if err = s.SaveMediaDetails(stale); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	stale.Revision = ""
	if err = s.SaveMediaDetails(stale); !errors.Is(err, ErrConflict) {
		t.Fatal("missing revision accepted", err)
	}
	current, err := s.MediaFile(first.Name)
	if err != nil || current.Title != first.Title {
		t.Fatal("original overwritten", err)
	}
	current.Alt = "New alt"
	if err = s.SaveMediaDetails(current); err != nil {
		t.Fatal(err)
	}
}

func TestMediaProtectsSEORevisionsSettingsAndModules(t *testing.T) {
	for _, kind := range []string{"draft", "published", "revision", "default", "logo", "module", "external"} {
		t.Run(kind, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()
			if err = s.Install(ctx, Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
				t.Fatal(err)
			}
			if err = s.Media("photo.png", "image/png", 123, 1); err != nil {
				t.Fatal(err)
			}
			image := "https://example.com/uploads/photo.png?v=2#preview"
			switch kind {
			case "draft", "published", "revision", "external":
				if kind == "draft" {
					image = "/uploads/photo.png?v=2"
				}
				if kind == "external" {
					image = "https://external.test/uploads/photo.png"
				}
				p, e := s.Save(ctx, Page{Title: "Page", Slug: "page", Document: lifecycleDocument, SEO: SEO{SocialImage: image}}, kind != "draft")
				if e != nil {
					t.Fatal(e)
				}
				if kind == "published" || kind == "revision" {
					p.SEO = SEO{}
					if _, e = s.Save(ctx, p, kind == "revision"); e != nil {
						t.Fatal(e)
					}
				}
			case "default", "logo":
				a, e := s.Appearance()
				if e != nil {
					t.Fatal(e)
				}
				if kind == "default" {
					a.SEOOptions.DefaultImage = image
				} else {
					a.SEOOptions.LogoURL = image
				}
				if e = s.SaveAppearance(ctx, a); e != nil {
					t.Fatal(e)
				}
			case "module":
				if err = s.InstallModulePackage(ctx, ModulePackage{ID: "contact", Manifest: `{}`, Template: "Hello", Settings: `{"image":"` + image + `"}`}); err != nil {
					t.Fatal(err)
				}
			}
			protected := kind != "external"
			used, e := s.MediaUsed("photo.png")
			if e != nil || used != protected {
				t.Fatal("protection", used, e)
			}
			e = s.DeleteMedia(ctx, "photo.png")
			if protected && !errors.Is(e, ErrConflict) || !protected && e != nil {
				t.Fatal("deletion protection", e)
			}
		})
	}
}
