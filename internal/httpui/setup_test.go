package httpui

import (
	"context"
	"gopress/internal/i18n"
	"gopress/internal/store"
	"html"
	"strings"
	"testing"
)

func TestSetupChecklistUsesActualPublicationState(t *testing.T) {
	_, app, b := seoApp(t)
	response := b.request("GET", "/admin/setup", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Не завершено") {
		t.Fatal("empty installation marked complete")
	}
	p, err := app.Service.Save(context.Background(), store.Page{Kind: "post", Title: "Published", Slug: "setup-post", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = p
	response = b.request("GET", "/admin/setup", "", "", false)
	status(t, response, 200)
	if strings.Count(response.Body.String(), "Не завершено") != 1 {
		t.Fatal("publication state not reflected")
	}
}

func TestSetupChecklistUsesSelectedLanguage(t *testing.T) {
	s, app, b := seoApp(t)
	sources := []string{"Site name and address", "Available template selected", "First content published", "Homepage has published content", "Backup exists"}
	for _, language := range []string{"en", "ru", "pl", "de", "uk", "it", "es", "cs", "zh", "ko", "ja", "fr", "pt", "tr"} {
		t.Run(language, func(t *testing.T) {
			settings, err := s.Settings()
			if err != nil {
				t.Fatal(err)
			}
			settings.Language = language
			if err = app.Service.SaveSettings(context.Background(), settings); err != nil {
				t.Fatal(err)
			}
			response := b.request("GET", "/admin/setup", "", "", false)
			status(t, response, 200)
			body := html.UnescapeString(response.Body.String())
			ctx := i18n.WithLanguage(context.Background(), language)
			for _, source := range sources {
				if language != "en" && i18n.Catalog(language)[source] == "" {
					t.Fatal("missing checklist translation", source)
				}
				if !strings.Contains(body, i18n.T(ctx, source)) {
					t.Fatal("setup label is not localized", source)
				}
			}
			if language != "ru" && strings.Contains(body, "Название и адрес сайта") {
				t.Fatal("Russian source label leaked into another language")
			}
		})
	}
}
