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

func TestAdminHTMLForms(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	if err := app.Service.Install(context.Background(), app.Service.InstallToken, store.Site{Title: "My Site", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	login := url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}
	status(t, b.request("POST", "/login", login.Encode(), "application/x-www-form-urlencoded", true), 303)
	// Browser requests receive HTML; existing JSON callers keep their response format.
	r := httptest.NewRequest("GET", "/admin/pages", nil)
	r.Header.Set("Accept", "text/html")
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	app.Echo.ServeHTTP(w, r)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "Начните с первой страницы") || !strings.Contains(w.Body.String(), "/assets/htmx.min.js") || !strings.Contains(w.Header().Get("Vary"), "Accept") {
		t.Fatal("missing admin shell")
	}
	status(t, b.request("GET", "/admin/pages/new", "", "", false), 200)
	w = b.request("GET", "/admin/pages", "", "", false)
	status(t, w, 200)
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("JSON compatibility changed")
	}
	fields := url.Values{"id": {"0"}, "version": {"0"}, "title": {"A page"}, "slug": {"form-page"}, "document": {document}, "cover": {""}, "intent": {"publish"}}
	status(t, b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", false), 403)
	w = b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, w, 303)
	pages, _, err := s.ListContent("page", "", false, 100, 0)
	if err != nil || len(pages) != 1 {
		t.Fatalf("pages: %v", err)
	}
	p := pages[0]
	if !p.Published || p.PublishedSlug != "form-page" || p.Version != 2 {
		t.Fatalf("save and publish: %+v", p)
	}
	fields.Set("id", fmt.Sprint(p.ID))
	fields.Set("version", fmt.Sprint(p.Version))
	fields.Set("title", "PRIVATE TITLE")
	fields.Set("slug", "private-address")
	fields.Set("intent", "save")
	w = b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, w, 303)
	public, err := s.Public("form-page")
	if err != nil || public.Title != "A page" {
		t.Fatal("form save leaked draft")
	}
	// Stale submit must preserve the posted text and title in the returned editor.
	fields.Set("title", "MY UNSAVED TITLE")
	fields.Set("document", strings.ReplaceAll(document, "First public revision", "MY UNSAVED BODY"))
	w = b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, w, 409)
	for _, text := range []string{"MY UNSAVED TITLE", "MY UNSAVED BODY", "Страница изменена", "href=\"/form-page\""} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("conflict lost %s", text)
		}
	}
	current, err := s.Page(p.ID)
	if err != nil || current.Title != "PRIVATE TITLE" {
		t.Fatal("stale submit overwrote content")
	}
	fields.Set("version", fmt.Sprint(current.Version))
	fields.Set("title", "INVALID BODY")
	fields.Set("document", `{"type":"doc","content":[{"type":"script","text":"</textarea><script>alert(1)</script>"}]}`)
	w = b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, w, 400)
	if strings.Contains(w.Body.String(), "<script>alert(1)") {
		t.Fatal("invalid document escaped textarea")
	}
	unchanged, err := s.Page(p.ID)
	if err != nil || unchanged.Version != current.Version {
		t.Fatal("invalid input modified page")
	}

	if _, err = app.Service.Save(context.Background(), store.Page{Title: "Dashboard draft", Slug: "dashboard-draft", Document: document}, false); err != nil {
		t.Fatal(err)
	}
	summary, err := s.PageSummary()
	if err != nil || summary.Total != 2 || summary.Published != 1 || summary.Drafts != 1 {
		t.Fatalf("dashboard counts: %+v %v", summary, err)
	}
	w = b.request("GET", "/admin", "", "", false)
	status(t, w, 200)
	for _, marker := range []string{`data-slot="sidebar-wrapper"`, `data-slot="sidebar-inset"`, `class="gp-dashboard-grid"`, `class="dark"`, "Последние изменения", `data-slot="table"`, `/assets/components.bundle.js`, "Dashboard draft", "PRIVATE TITLE"} {
		if !strings.Contains(w.Body.String(), marker) {
			t.Fatalf("missing dashboard content: %s", marker)
		}
	}
	for _, removed := range []string{"Следующий шаг", "От идеи к публикации", "Ваше пространство", "Управление контентом", "workspace-label"} {
		if strings.Contains(w.Body.String(), removed) {
			t.Fatalf("dashboard still contains decoration: %s", removed)
		}
	}
	if strings.Contains(w.Body.String(), " style=") {
		t.Fatal("dashboard uses inline styles incompatible with CSP")
	}

	// Cover selections are server-rendered HTML fragments, never the editor DOM.
	if err = s.Media("chosen.png", "image/png", 10); err != nil {
		t.Fatal(err)
	}
	w = b.request("GET", "/admin/media/choices?selected=chosen.png", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "value=\"chosen.png\"") || !strings.Contains(w.Body.String(), "checked") || strings.Contains(w.Body.String(), "<html") {
		t.Fatal("bad media fragment")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private fragment is cacheable")
	}
	anon := httptest.NewRequest("GET", "/admin/pages", nil)
	anon.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	app.Echo.ServeHTTP(w, anon)
	status(t, w, 303)
	if w.Header().Get("Location") != "/login" {
		t.Fatal("anonymous browser not sent to login")
	}
	for _, asset := range []string{"admin.css", "admin.js", "htmx.min.js", "editor.bundle.js", "frontend-LICENSES.txt", "components.bundle.js"} {
		w = b.request("GET", "/assets/"+asset, "", "", false)
		status(t, w, 200)
	}
}
func TestAtomicFormPublishRollsBackOnConflict(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Title: "Published", Slug: "original", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	original := p
	p.Title = "Winning draft"
	p, err = app.Service.Save(ctx, p, false)
	if err != nil {
		t.Fatal(err)
	}
	original.Title = "STALE PUBLICATION"
	if _, err = app.Service.Save(ctx, original, true); err != store.ErrConflict {
		t.Fatalf("wanted conflict: %v", err)
	}
	public, err := s.Public("original")
	if err != nil || public.Title != "Published" {
		t.Fatal("stale publish changed public snapshot")
	}
	saved, err := s.Page(p.ID)
	if err != nil || saved.Title != "Winning draft" {
		t.Fatal("stale publish changed draft")
	}
}

func TestPostEditorAndPublicTheme(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Journal", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	w := b.request("GET", "/admin/posts/new", "", "", false)
	status(t, w, 200)
	for _, marker := range []string{"Новая запись", `name="kind" value="post"`, `data-templ-collapsible="icon"`} {
		if !strings.Contains(w.Body.String(), marker) {
			t.Fatalf("missing %s", marker)
		}
	}
	fields := url.Values{"id": {"0"}, "version": {"0"}, "kind": {"post"}, "title": {"Public story"}, "slug": {"story"}, "document": {document}, "intent": {"publish"}}
	status(t, b.request("POST", "/admin/pages/save", fields.Encode(), "application/x-www-form-urlencoded", true), 303)
	posts, _, err := s.ListContent("post", "", false, 100, 0)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts %v", err)
	}
	p, err := s.Page(posts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "PRIVATE STORY"
	p.Slug = "private-story"
	if _, err = app.Service.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	w = b.request("GET", "/", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "Public story") || strings.Contains(w.Body.String(), "PRIVATE STORY") {
		t.Fatal("home leaked draft or omitted published story")
	}
	w = b.request("GET", "/admin", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "PRIVATE STORY") || !strings.Contains(w.Body.String(), "Последние изменения") {
		t.Fatal("dashboard omitted posts")
	}
	w = b.request("GET", "/story", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "Public story") {
		t.Fatal("post unavailable")
	}
}
