package httpui

import (
	"context"
	"fmt"
	"gopress/internal/store"
	"net/url"
	"strings"
	"testing"
)

func TestManualRedirectsFollowPublishedTargetsAndReservePaths(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Title: "Target", Slug: "target", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"slug": {"/old"}, "page_id": {fmt.Sprint(p.ID)}}
	status(t, adminHTML(b, "POST", "/admin/seo/redirects", values, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/seo/redirects", values, false, false, true), 303)
	response := b.request("GET", "/old?lang=en", "", "", false)
	status(t, response, 301)
	if response.Header().Get("Location") != "/target?lang=en" {
		t.Fatal(response.Header())
	}
	p.Slug = "renamed"
	p, err = app.Service.Save(ctx, p, true)
	if err != nil {
		t.Fatal(err)
	}
	response = b.request("GET", "/old", "", "", false)
	status(t, response, 301)
	if response.Header().Get("Location") != "/renamed" {
		t.Fatal(response.Header())
	}
	if _, err = app.Service.Save(ctx, store.Page{Title: "Collision", Slug: "old", Document: document}, false); err == nil {
		t.Fatal("redirect path reused")
	}
	for _, slug := range []string{"/renamed", "/admin/path", "//example.com", "/x?secret=1", "/../x"} {
		values.Set("slug", slug)
		response = adminHTML(b, "POST", "/admin/seo/redirects", values, false, false, true)
		if response.Code < 400 {
			t.Fatal("invalid path", slug, response.Code)
		}
	}
	status(t, b.request("GET", "/unknown-path?secret=private", "", "", false), 404)
	status(t, b.request("GET", "/unknown-path", "", "", false), 404)
	missing, err := s.MissingPaths()
	if err != nil || len(missing) != 1 || missing[0].Slug != "unknown-path" || missing[0].Hits != 2 {
		t.Fatal(missing, err)
	}
	response = b.request("GET", "/admin/seo/redirects", "", "", false)
	status(t, response, 200)
	if strings.Contains(response.Body.String(), "secret=private") {
		t.Fatal("query leaked")
	}
	values.Set("slug", "old")
	values.Set("action", "delete")
	status(t, adminHTML(b, "POST", "/admin/seo/redirects", values, false, false, true), 303)
	status(t, b.request("GET", "/old", "", "", false), 404)
}
