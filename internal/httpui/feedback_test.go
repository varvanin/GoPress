package httpui

import (
	"context"
	"html"
	"strings"
	"testing"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

func TestAdminFeedbackSurvivesNavigation(t *testing.T) {
	_, _, b := seoApp(t)
	full := b.request("GET", "/admin", "", "", false)
	status(t, full, 200)
	for _, marker := range []string{`id="system-toasts"`, `role="alertdialog"`, `data-confirm-result="cancel"`, `data-confirm-result="accept"`, `data-templ-disable-pointer-dismissal="true"`} {
		if strings.Count(full.Body.String(), marker) != 1 {
			t.Errorf("feedback shell must contain one %s", marker)
		}
	}
	partial := adminHTML(b, "GET", "/admin/themes", nil, true, false, false)
	status(t, partial, 200)
	if strings.Contains(partial.Body.String(), `id="system-toasts"`) || strings.Contains(partial.Body.String(), `id="system-confirm"`) {
		t.Fatal("SPA partial duplicated persistent feedback shell")
	}
}

func TestFeedbackUsesSelectedLanguage(t *testing.T) {
	s, app, b := seoApp(t)
	for _, language := range []string{"en", "ru", "pl", "de", "uk", "it", "es", "cs", "zh", "ko", "ja", "fr", "pt", "tr"} {
		settings, err := s.Settings()
		if err != nil {
			t.Fatal(err)
		}
		settings.Language = language
		if err := app.Service.SaveSettings(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
		ctx := i18n.WithLanguage(context.Background(), language)
		var media strings.Builder
		if err := MediaLibrary("Site", "https://example.com", "token", nil, 0, 1, "", "", &store.MediaFile{Name: "sample.png"}, false, false).Render(ctx, &media); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.UnescapeString(media.String()), `hx-confirm="`+i18n.T(ctx, "Permanently delete this image?")+`"`) {
			t.Fatal("untranslated image confirmation", language)
		}
		for path, message := range map[string]string{
			"/admin/settings?tab=general&saved=1": "Settings saved.",
			"/admin/themes?saved=1":               "Template activated.",
			"/admin/media?uploaded=1":             "Image uploaded.",
		} {
			response := adminHTML(b, "GET", path, nil, false, false, false)
			status(t, response, 200)
			body := html.UnescapeString(response.Body.String())
			if !strings.Contains(body, i18n.T(ctx, message)) {
				t.Fatal("untranslated notice", language, path)
			}
			for _, label := range []string{"Confirm action", "Cancel", "Delete"} {
				if !strings.Contains(body, i18n.T(ctx, label)) {
					t.Fatal("untranslated dialog", language, label)
				}
			}
		}
	}
}
