package httpui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/store"
)

func adminHTML(b *browser, method, path string, fields url.Values, partial, history, csrf bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(fields.Encode()))
	r.Header.Set("Accept", "text/html")
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if partial {
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Boosted", "true")
	}
	if history {
		r.Header.Set("HX-History-Restore-Request", "true")
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", b.csrf)
	}
	for _, cookie := range b.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	b.app.Echo.ServeHTTP(w, r)
	return w
}

func TestSPAContentLifecycle(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "CMS", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	p, err := app.Service.Save(ctx, store.Page{Title: "Published story", Kind: "post", Slug: "story", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	original := p
	full := adminHTML(b, "GET", "/admin/posts", nil, false, false, false)
	status(t, full, 200)
	partial := adminHTML(b, "GET", "/admin/posts", nil, true, false, false)
	status(t, partial, 200)
	body := partial.Body.String()
	for _, marker := range []string{`id="main"`, "Published story", `data-section="/admin/posts"`, "<title>"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("fragment missing %s", marker)
		}
	}
	for _, forbidden := range []string{"<!DOCTYPE", `data-slot="sidebar-wrapper"`, "<script"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("fragment repeats shell: %s", forbidden)
		}
	}
	if len(body) >= full.Body.Len() {
		t.Fatal("fragment is not smaller than full page")
	}
	t.Logf("content HTML full=%d bytes partial=%d bytes", full.Body.Len(), len(body))
	if !strings.Contains(partial.Result().Header.Get("Cache-Control"), "no-store") || !strings.Contains(strings.Join(partial.Result().Header.Values("Vary"), ","), "HX-Request") {
		t.Fatal("incorrect fragment caching")
	}
	restored := adminHTML(b, "GET", "/admin/posts", nil, true, true, false)
	status(t, restored, 200)
	if !strings.Contains(restored.Body.String(), `data-slot="sidebar-wrapper"`) {
		t.Fatal("history restore needs full document")
	}

	// A stale SPA submit returns the attempted fields, not a generic replacement screen.
	p.Title = "Winning draft"
	p, err = app.Service.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"id": {fmt.Sprint(p.ID)}, "version": {fmt.Sprint(original.Version)}, "kind": {"post"}, "title": {"MY UNSAVED TEXT"}, "slug": {"story"}, "document": {document}, "intent": {"save"}}
	conflict := adminHTML(b, "POST", "/admin/pages/save", fields, true, false, true)
	status(t, conflict, 409)
	if !strings.Contains(conflict.Body.String(), "MY UNSAVED TEXT") || !strings.Contains(conflict.Body.String(), `id="main"`) || strings.Contains(conflict.Body.String(), "<script") {
		t.Fatal("conflict lost SPA editor")
	}
	history := adminHTML(b, "GET", fmt.Sprintf("/admin/pages/%d/revisions/%d", p.ID, original.Version), nil, false, false, false)
	status(t, history, 200)
	if !strings.Contains(history.Body.String(), "Published story") {
		t.Fatal("revision preview lost old title")
	}
	revisionFields := url.Values{"version": {fmt.Sprint(p.Version)}}
	status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/revisions/%d/restore", p.ID, original.Version), revisionFields, true, false, false), 403)
	status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/revisions/%d/restore", p.ID, original.Version), revisionFields, true, false, true), 303)
	p, err = s.Page(p.ID)
	if err != nil || p.Title != original.Title {
		t.Fatal("revision restore failed")
	}

	for _, action := range []string{"unpublish", "trash", "restore"} {
		state := url.Values{"version": {fmt.Sprint(p.Version)}, "action": {action}}
		status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/state", p.ID), state, true, false, true), 303)
		p, err = s.Page(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Published {
			t.Fatal("state change unexpectedly publishes")
		}
		status(t, b.request("GET", "/story", "", "", false), 404)
		if action == "trash" {
			active := adminHTML(b, "GET", "/admin/posts", nil, true, false, false)
			if strings.Contains(active.Body.String(), `class="page-title"`) {
				t.Fatal("trash shown in active list")
			}
			trash := adminHTML(b, "GET", "/admin/posts?trash=1", nil, true, false, false)
			if !strings.Contains(trash.Body.String(), p.Title) {
				t.Fatal("trash missing from list")
			}
			editor := adminHTML(b, "GET", fmt.Sprintf("/admin/pages/%d/edit", p.ID), nil, true, false, false)
			if !strings.Contains(editor.Body.String(), `data-readonly="true"`) {
				t.Fatal("trash editor must be readonly")
			}
		}
	}
	status(t, adminHTML(b, "GET", "/admin/posts?q=Published", nil, true, false, false), 200)
	status(t, adminHTML(b, "GET", "/admin/posts?page=-1", nil, true, false, false), 400)
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	expired := adminHTML(anon, "GET", "/admin/posts", nil, true, false, false)
	status(t, expired, 204)
	if expired.Header().Get("HX-Redirect") != "/login" {
		t.Fatal("expired SPA session did not redirect")
	}
}

func TestBulkContentRoutes(t *testing.T) {
	s, app, b := seoApp(t)
	for _, kind := range []string{"page", "post"} {
		p, err := app.Service.Save(context.Background(), store.Page{Kind: kind, Title: "Bulk", Slug: "bulk-" + kind, Document: document}, false)
		if err != nil {
			t.Fatal(err)
		}
		list := adminHTML(b, "GET", contentPath(kind), nil, true, false, false)
		if !strings.Contains(list.Body.String(), `data-select-all`) || !strings.Contains(list.Body.String(), `form="content-bulk"`) {
			t.Fatal("missing selection")
		}
		fields := url.Values{"kind": {kind}, "action": {"trash"}, "selection": {fmt.Sprintf("%d:%d", p.ID, p.Version)}}
		status(t, adminHTML(b, "POST", "/admin/content/bulk", fields, true, false, false), 403)
		fields.Set("action", "publish")
		status(t, adminHTML(b, "POST", "/admin/content/bulk", fields, true, false, true), 400)
		fields.Set("action", "trash")
		status(t, adminHTML(b, "POST", "/admin/content/bulk", fields, true, false, true), 303)
		p, _ = s.Page(p.ID)
		trash := adminHTML(b, "GET", contentPath(kind)+"?trash=1", nil, true, false, false)
		if !strings.Contains(trash.Body.String(), "Удалить навсегда") {
			t.Fatal("missing permanent deletion")
		}
		fields.Set("selection", fmt.Sprintf("%d:%d", p.ID, p.Version))
		fields.Set("action", "restore")
		status(t, adminHTML(b, "POST", "/admin/content/bulk", fields, true, false, true), 303)
		p, _ = s.Page(p.ID)
		state := url.Values{"action": {"trash"}, "version": {fmt.Sprint(p.Version)}}
		status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/state", p.ID), state, true, false, true), 303)
		p, _ = s.Page(p.ID)
		state.Set("version", fmt.Sprint(p.Version))
		state.Set("action", "delete")
		status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/state", p.ID), state, true, false, true), 303)
	}
}
