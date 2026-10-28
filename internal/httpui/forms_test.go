package httpui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"gopress/internal/service"
	"gopress/internal/store"
)

func TestFormsBuilderAndPublicSubmission(t *testing.T) {
	t.Setenv("GOPRESS_SMTP_HOST", "")
	t.Setenv("GOPRESS_SMTP_FROM", "")
	s, app, b := seoApp(t)
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("GET", "/admin/forms", "", "", false), 401)
	fields := url.Values{"id": {"0"}, "version": {"0"}, "name": {"Contact <script>"}, "recipient": {"owner@example.com"}, "button": {"Send"}, "success": {"Thanks"}, "enabled": {"true"}, "field_label": {"Email", "Topic", "Agree"}, "field_type": {"email", "select", "checkbox"}, "field_required": {"true", "true", "true"}, "field_options": {"", "Sales\nSupport", ""}}
	status(t, adminHTML(b, "POST", "/admin/forms/save", fields, true, false, false), 403)
	response := adminHTML(b, "POST", "/admin/forms/save", fields, true, false, true)
	status(t, response, 303)
	forms, err := s.Forms()
	if err != nil || len(forms) != 1 {
		t.Fatal(forms, err)
	}
	f := forms[0]
	path := fmt.Sprintf("/forms/%d", f.ID)
	editor := adminHTML(b, "GET", formPath(f.ID)+"/edit", nil, false, false, false)
	status(t, editor, 200)
	if !strings.Contains(editor.Body.String(), formCode(f.ID)) || !strings.Contains(editor.Body.String(), "data-form-builder") || strings.Contains(editor.Body.String(), "Contact <script>") {
		t.Fatal("builder missing or unsafe")
	}
	public := anon.request("GET", path, "", "", false)
	status(t, public, 200)
	if strings.Contains(public.Body.String(), f.Recipient) || !strings.Contains(public.Body.String(), `value="Support"`) {
		t.Fatal("public form leaks settings or lacks select options")
	}
	if public.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("CSRF form cached")
	}
	posted := url.Values{"submission_key": {strings.Repeat("a", 64)}, "version": {fmt.Sprint(f.Version)}, "field_0": {"visitor@example.com"}, "field_1": {"Sales"}, "field_2": {"Да"}}
	status(t, anon.request("POST", path, posted.Encode(), "application/x-www-form-urlencoded", false), 403)
	posted.Set("field_0", "invalid")
	invalid := anon.request("POST", path, posted.Encode(), "application/x-www-form-urlencoded", true)
	status(t, invalid, 400)
	if !strings.Contains(invalid.Body.String(), `value="Sales" selected`) {
		t.Fatal("validation lost selected values")
	}
	posted.Set("field_0", "visitor@example.com")
	posted.Set("website", "bot")
	status(t, anon.request("POST", path, posted.Encode(), "application/x-www-form-urlencoded", true), 303)
	_, total, err := s.FormEntries(f.ID, 1)
	if err != nil || total != 0 {
		t.Fatal("honeypot stored entry", err)
	}
	posted.Del("website")
	status(t, anon.request("POST", path, posted.Encode(), "application/x-www-form-urlencoded", true), 303)
	entries, total, err := s.FormEntries(f.ID, 1)
	if err != nil || total != 1 || entries[0].MailStatus != "pending" {
		t.Fatal(entries, total, err)
	}
	success := anon.request("GET", path+"?sent=1", "", "", false)
	status(t, success, 200)
	if !strings.Contains(success.Body.String(), "Thanks") {
		t.Fatal("no success message")
	}
	inbox := adminHTML(b, "GET", formPath(f.ID)+"/entries", nil, false, false, false)
	status(t, inbox, 200)
	if !strings.Contains(inbox.Body.String(), "visitor@example.com") || !strings.Contains(inbox.Body.String(), "Уведомление поставлено в очередь.") {
		t.Fatal("inbox missing entry or queue status")
	}
	status(t, adminHTML(b, "POST", fmt.Sprintf("%s/entries/%d/retry", formPath(f.ID), entries[0].ID), nil, true, false, true), 303)
	// Two embeds on a visitor's first request must share one CSRF cookie/token.
	doc := service.Node{Type: "doc", Content: []service.Node{{Type: "paragraph", Content: []service.Node{{Type: "text", Text: formCode(f.ID)}}}, {Type: "paragraph", Content: []service.Node{{Type: "text", Text: formCode(f.ID)}}}}}
	raw, _ := json.Marshal(doc)
	p, err := app.Service.Save(context.Background(), store.Page{Title: "Forms", Slug: "contact-forms", Document: string(raw)}, true)
	if err != nil {
		t.Fatal(err)
	}
	visitor := &browser{app: app, cookies: map[string]*http.Cookie{}}
	embedded := visitor.request("GET", "/"+p.Slug, "", "", false)
	status(t, embedded, 200)
	tokens := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindAllStringSubmatch(embedded.Body.String(), -1)
	if len(tokens) != 2 || tokens[0][1] != tokens[1][1] || tokens[0][1] != visitor.csrf {
		t.Fatal("embedded CSRF tokens do not match cookie", len(tokens))
	}
	if strings.Contains(embedded.Body.String(), formCode(f.ID)) {
		t.Fatal("shortcode not expanded")
	}
	f.Enabled = false
	if _, err = app.Service.SaveForm(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	status(t, anon.request("GET", path, "", "", false), 404)
	status(t, anon.request("POST", path, posted.Encode(), "application/x-www-form-urlencoded", true), 404)
}
