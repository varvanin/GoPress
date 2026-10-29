package httpui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestEditorTranslationsSaveAsOneMaterialAndKeepDraftsPrivate(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Site", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	for _, kind := range []string{"page", "post"} {
		t.Run(kind, func(t *testing.T) {
			original := store.Translation{Title: "Original", Document: document}
			translations := map[string]store.Translation{"en": {Title: "Hello", Document: document, SEO: store.SEO{Description: "English description"}}}
			form := url.Values{"id": {"0"}, "version": {"0"}, "kind": {kind}, "title": {"Hello"}, "slug": {"translation-" + kind}, "document": {document}, "seo_description": {"English description"}, "content_language": {"en"}, "source_language": {"de"}, "original_translation": {translationJSON(original)}, "translations": {translationJSON(translations)}, "intent": {"autosave"}}
			response := b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true)
			status(t, response, 200)
			var saved struct{ ID, Version int64 }
			if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
				t.Fatal(err)
			}
			p, err := s.Page(saved.ID)
			if err != nil || p.Title != "Original" || p.Translations["en"].Title != "Hello" || p.SEO.Language != "de" {
				t.Fatal(p, err)
			}
			status(t, b.request("GET", fmt.Sprintf("/admin/pages/%d/preview?lang=en", p.ID), "", "", false), 200)
			if err = s.Publish(ctx, p.ID, p.Version); err != nil {
				t.Fatal(err)
			}
			originalResponse := b.request("GET", "/translation-"+kind, "", "", false)
			status(t, originalResponse, 200)
			if !strings.Contains(originalResponse.Body.String(), `lang="de"`) {
				t.Fatal("original language follows the admin instead of the material")
			}
			response = b.request("GET", "/translation-"+kind+"?lang=en", "", "", false)
			status(t, response, 200)
			for _, want := range []string{`lang="en"`, "Hello", "English description", `http://example.com/translation-` + kind + `?lang=en`} {
				if !strings.Contains(response.Body.String(), want) {
					t.Fatal("missing public translation", want)
				}
			}
			status(t, b.request("GET", "/translation-"+kind+"?lang=pl", "", "", false), 404)
			p, err = s.Page(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			form.Set("id", fmt.Sprint(p.ID))
			form.Set("version", fmt.Sprint(p.Version))
			form.Set("title", "Private translation")
			status(t, b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true), 200)
			response = b.request("GET", "/translation-"+kind+"?lang=en", "", "", false)
			status(t, response, 200)
			if strings.Contains(response.Body.String(), "Private translation") {
				t.Fatal("draft translation leaked")
			}
			form.Set("version", "0")
			form.Set("intent", "save")
			response = b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true)
			status(t, response, 409)
			if !strings.Contains(response.Body.String(), "Private translation") {
				t.Fatal("conflict discarded translation")
			}
		})
	}
}

func TestMaterialLanguagesAreIndependentOfTheAdminAndAddedExplicitly(t *testing.T) {
	p := store.Page{SEO: store.SEO{Language: "de"}}
	items := contentLanguageOptions(p)
	if len(items) != 1 || items[0].Value != "original" || items[0].Label != "Deutsch" {
		t.Fatal(items)
	}
	p.Translations = map[string]store.Translation{"en": {Title: "English", Document: document}}
	items = contentLanguageOptions(p)
	if len(items) != 2 || items[1].Value != "en" {
		t.Fatal(items)
	}
	for _, candidate := range availableContentLanguages(p) {
		if candidate.Value == "de" || candidate.Value == "en" {
			t.Fatal("offers an existing language", candidate)
		}
	}
}

func TestLanguageControlRendersOnlyExistingVersions(t *testing.T) {
	for _, translations := range []map[string]store.Translation{nil, {"en": {Title: "English", Document: document}}} {
		p := store.Page{SEO: store.SEO{Language: "de"}, Translations: translations}
		var output strings.Builder
		if err := EditorContentLanguage(p, "original").Render(context.Background(), &output); err != nil {
			t.Fatal(err)
		}
		main := strings.Split(output.String(), `class="editor-add-language"`)[0]
		if count := strings.Count(main, `role="option"`); count != 1+len(translations) {
			t.Fatalf("language switcher has %d options for %d existing versions", count, 1+len(translations))
		}
		if strings.Contains(main, `id="source-language"`) {
			t.Fatal("renders the full source-language catalog as the version switcher")
		}
	}
}
