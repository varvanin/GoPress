package httpui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestDuplicateCreatesPrivateIndependentDraft(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	source, err := app.Service.Save(ctx, store.Page{Title: "Original", Slug: "source", Document: document, SEO: store.SEO{Canonical: "https://example.com/custom"}, Translations: map[string]store.Translation{"en": {Title: "English", Document: document, SEO: store.SEO{Canonical: "https://example.com/en"}}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"version": {fmt.Sprint(source.Version)}}
	route := fmt.Sprintf("/admin/pages/%d/duplicate", source.ID)
	status(t, adminHTML(b, "POST", route, values, false, false, false), 403)
	response := adminHTML(b, "POST", route, values, false, false, true)
	status(t, response, 303)
	var id int64
	if _, err = fmt.Sscanf(response.Header().Get("Location"), "/admin/pages/%d/edit", &id); err != nil || id == source.ID {
		t.Fatal("missing duplicate location", err)
	}
	copy, err := s.Page(id)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Published || copy.PublishedAt != 0 || copy.ScheduledAt != 0 || copy.Slug != "source-copy" || copy.Document != source.Document || copy.Translations["en"].Title != "English" || copy.SEO.Canonical != "" || copy.Translations["en"].SEO.Canonical != "" {
		t.Fatalf("invalid duplicate: %+v", copy)
	}
	original, err := s.Page(source.ID)
	if err != nil || original.SEO.Canonical != source.SEO.Canonical || !original.Published {
		t.Fatal("source modified", err)
	}
	values.Set("version", fmt.Sprint(source.Version-1))
	status(t, adminHTML(b, "POST", route, values, false, false, true), 409)
}

func TestBlockPickerOnlyListsEnabledContentWithoutPrivateFields(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	id, err := s.SaveForm(ctx, store.Form{Name: "Public contact", Recipient: "private-recipient@example.test", Button: "Send", Success: "Thanks", Enabled: true, Fields: []store.FormField{{Type: "text", Label: "Private field definition"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveForm(ctx, store.Form{Name: "Disabled contact", Recipient: "owner@example.test", Button: "Send", Success: "Thanks", Enabled: false, Fields: []store.FormField{{Type: "text", Label: "Name"}}}); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/admin/pages/new", "", "", false)
	status(t, response, 200)
	body := response.Body.String()
	if !strings.Contains(body, "Public contact") || !strings.Contains(body, fmt.Sprintf("[form id=%d]", id)) || strings.Contains(body, "Disabled contact") || strings.Contains(body, "private-recipient@example.test") || strings.Contains(body, "Private field definition") {
		t.Fatal("picker missing enabled options or leaking form data")
	}
	if _, err = app.Service.SaveUser(ctx, store.User{Email: "author@example.test", Role: "author"}, "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	author := userBrowser(t, app, "author@example.test")
	source, err := app.Service.Save(ctx, store.Page{Kind: "post", Title: "Other user's post", Slug: "not-mine", Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(author, "POST", fmt.Sprintf("/admin/pages/%d/duplicate", source.ID), url.Values{"version": {fmt.Sprint(source.Version)}}, false, false, true), 403)
}
