package httpui

import (
	"context"
	"encoding/json"
	"errors"
	"gopress/internal/service"
	"gopress/internal/store"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFAQModuleLifecycleAndPublicEmbedding(t *testing.T) {
	s, app, b := seoApp(t)
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("GET", "/admin/modules", "", "", false), 401)
	status(t, b.request("GET", "/admin/modules", "", "", false), 200)
	fields := url.Values{"id": {"0"}, "version": {"0"}, "name": {"Delivery"}, "question": {"First <script>", "Second?"}, "answer": {"Answer <script>alert(1)</script>", "Line one\nLine two"}}
	status(t, adminHTML(b, "POST", "/admin/modules/faq/save", fields, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/modules/faq/save", fields, false, false, true), 303)
	groups, err := s.FAQGroups()
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	f := groups[0]
	status(t, b.request("GET", faqPath(f.ID), "", "", false), 200)
	doc := service.Node{Type: "doc", Content: []service.Node{{Type: "paragraph", Content: []service.Node{{Type: "text", Text: faqCode(f.ID)}}}}}
	raw, _ := json.Marshal(doc)
	p, err := app.Service.Save(context.Background(), store.Page{Title: "FAQ page", Slug: "faq-page", Document: string(raw)}, true)
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		r := anon.request("GET", "/"+p.Slug, "", "", false)
		status(t, r, 200)
		return r.Body.String()
	}
	if body := read(); strings.Contains(body, "gp-faq") || strings.Contains(body, faqCode(f.ID)) {
		t.Fatal("disabled FAQ visible")
	}
	status(t, adminHTML(b, "POST", "/admin/modules/faq/toggle", url.Values{"enabled": {"true"}}, false, false, true), 303)
	body := read()
	if !strings.Contains(body, "gp-faq") || !strings.Contains(body, "<details>") || strings.Contains(body, "<script>alert(1)</script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("FAQ missing or unsafe")
	}
	if strings.Index(body, "First &lt;script&gt;") > strings.Index(body, "Second?") {
		t.Fatal("wrong order")
	}
	if _, err = app.Service.SaveFAQ(context.Background(), store.FAQGroup{ID: f.ID, Version: 0, Name: "stale", Items: f.Items}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	for _, role := range []string{"editor", "author"} {
		ctx := store.WithActor(context.Background(), store.User{Role: role})
		if _, err = app.Service.SaveFAQ(ctx, f); !errors.Is(err, service.ErrForbidden) {
			t.Fatal("role allowed", role, err)
		}
	}
	status(t, adminHTML(b, "POST", "/admin/modules/faq/toggle", url.Values{"enabled": {"false"}}, false, false, true), 303)
	if strings.Contains(read(), "gp-faq") {
		t.Fatal("disable failed")
	}
	if _, err = s.FAQ(f.ID); err != nil {
		t.Fatal("disabled data lost", err)
	}
	status(t, adminHTML(b, "POST", "/admin/modules/faq/delete", url.Values{"id": {"1"}, "version": {"0"}}, false, false, true), 400)
	status(t, adminHTML(b, "POST", "/admin/modules/faq/delete", url.Values{"id": {"1"}, "version": {"1"}}, false, false, true), 303)
	groups, err = s.FAQGroups()
	if err != nil || len(groups) != 0 {
		t.Fatal(groups, err)
	}
}
func TestFAQValidation(t *testing.T) {
	_, app, _ := seoApp(t)
	for _, f := range []store.FAQGroup{{Name: "Empty"}, {Name: "Missing answer", Items: []store.FAQItem{{Question: "Q"}}}, {Name: "Long", Items: []store.FAQItem{{Question: strings.Repeat("q", 301), Answer: "A"}}}} {
		if _, err := app.Service.SaveFAQ(context.Background(), f); !errors.Is(err, service.ErrInvalid) {
			t.Fatal("invalid FAQ accepted", err)
		}
	}
}
