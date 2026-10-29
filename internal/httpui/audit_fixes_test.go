package httpui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"gopress/internal/store"
)

func TestContentRedirectsPreserveLanguageAndQuery(t *testing.T) {
	_, app, b := seoApp(t)
	p, err := app.Service.Save(context.Background(), store.Page{Title: "Original", Slug: "before", Document: document, Translations: map[string]store.Translation{"en": {Title: "English version", Document: document}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.Slug = "after"
	if _, err = app.Service.Save(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/before?lang=en&q=test", "/AFTER?lang=en&q=test"} {
		response := b.request("GET", path, "", "", false)
		status(t, response, 301)
		if got := response.Header().Get("Location"); got != "/after?lang=en&q=test" {
			t.Fatal(got)
		}
		response = b.request("GET", response.Header().Get("Location"), "", "", false)
		status(t, response, 200)
		if !strings.Contains(response.Body.String(), "English version") {
			t.Fatal("redirect lost the translation")
		}
	}
}

func TestPublicFormLimitPrecedesBothCSRFModes(t *testing.T) {
	s, app, b := seoApp(t)
	id, err := app.Service.SaveForm(context.Background(), store.Form{Name: "Contact", Recipient: "owner@example.com", Button: "Send", Success: "Thanks", Enabled: true, Fields: []store.FormField{{Label: "Message", Type: "text"}}})
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"csrf": {b.csrf}, "version": {"1"}, "submission_key": {strings.Repeat("a", 64)}, "field_0": {"hello"}, "padding": {strings.Repeat("x", 128<<10)}}
	path := fmt.Sprintf("/forms/%d", id)
	for _, header := range []bool{false, true} {
		response := b.request("POST", path, values.Encode(), "application/x-www-form-urlencoded", header)
		if response.Code < 400 {
			t.Fatalf("accepted an oversized form, header=%v", header)
		}
	}
	_, total, err := s.FormEntries(id, 1)
	if err != nil || total != 0 {
		t.Fatal(total, err)
	}
	values.Del("padding")
	status(t, b.request("POST", path, values.Encode(), "application/x-www-form-urlencoded", false), 303)
}

func TestLargeMultilingualContentWorksThroughFormAndJSON(t *testing.T) {
	s, _, b := seoApp(t)
	raw, err := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": strings.Repeat("я", 480*1024)}}}}})
	if err != nil {
		t.Fatal(err)
	}
	p := store.Page{Title: "Large", Slug: "large-form", Document: string(raw), Translations: map[string]store.Translation{}}
	for _, code := range []string{"en", "pl", "de"} {
		p.Translations[code] = store.Translation{Title: code, Document: string(raw)}
	}
	values := url.Values{"id": {"0"}, "version": {"0"}, "title": {p.Title}, "slug": {p.Slug}, "document": {p.Document}, "translations": {translationJSON(p.Translations)}, "content_language": {"original"}, "source_language": {"ru"}, "intent": {"autosave"}}
	body := values.Encode()
	if len(body) <= 9<<20 {
		t.Fatal("fixture does not exercise the former transport limit")
	}
	response := b.request("POST", "/admin/pages/save", body, "application/x-www-form-urlencoded", true)
	status(t, response, 200)
	var saved store.Page
	if err = json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Page(saved.ID)
	if err != nil || len(loaded.Translations) != 3 {
		t.Fatal(err)
	}
	p.Slug = "large-json"
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	status(t, b.request("POST", "/admin/pages", string(data), "application/json", true), 200)
}

func TestLanguagePanelUsesOnlyLanguageMetadata(t *testing.T) {
	_, _, b := seoApp(t)
	values := url.Values{"source_language": {"ru"}, "content_language": {"en"}, "languages": {"en"}}
	response := b.request("POST", "/admin/pages/languages", values.Encode(), "application/x-www-form-urlencoded", true)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "English") || !strings.Contains(response.Body.String(), "Русский") {
		t.Fatal("missing languages")
	}
	values.Set("languages", "../en")
	status(t, b.request("POST", "/admin/pages/languages", values.Encode(), "application/x-www-form-urlencoded", true), 400)
}

func TestBackupImportDoesNotBlockPublicRequests(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := app.operationGuard(func(c *echo.Context) error {
		if c.Request().URL.Path == "/admin/tools/backups/import" {
			close(started)
			<-release
		}
		return c.NoContent(http.StatusNoContent)
	})
	go func() {
		defer close(finished)
		_ = handler(app.Echo.NewContext(httptest.NewRequest("POST", "/admin/tools/backups/import", nil), httptest.NewRecorder()))
	}()
	defer func() { close(release); <-finished }()
	<-started
	done := make(chan error, 1)
	go func() {
		done <- handler(app.Echo.NewContext(httptest.NewRequest("GET", "/healthz", nil), httptest.NewRecorder()))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backup import blocked a public request")
	}
}
