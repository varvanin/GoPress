package httpui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopress/internal/store"
)

func TestBlogTermsAutosaveScheduleAndPublicFilters(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Blog", BaseURL: "http://example.com", Timezone: "Europe/Warsaw"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	term := url.Values{"id": {"0"}, "version": {"0"}, "kind": {"category"}, "name": {"Новости"}, "action": {"save"}}
	status(t, adminHTML(b, "POST", "/admin/posts/terms", term, true, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/posts/terms", term, true, false, true), 303)
	terms, err := s.Terms(false)
	if err != nil || len(terms) != 1 {
		t.Fatal(err)
	}
	taxonomy := terms[0]
	form := url.Values{"id": {"0"}, "version": {"0"}, "kind": {"post"}, "title": {""}, "slug": {""}, "document": {document}, "intent": {"autosave"}, "term_ids": {fmt.Sprint(taxonomy.ID)}}
	response := b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true)
	status(t, response, 200)
	var saved struct {
		ID, Version int64
		Slug        string
	}
	if err = json.Unmarshal(response.Body.Bytes(), &saved); err != nil || saved.ID == 0 || saved.Version == 0 || !strings.HasPrefix(saved.Slug, "draft-") {
		t.Fatalf("new autosave %+v %v", saved, err)
	}
	p, err := s.Page(saved.ID)
	if err != nil || p.Published || p.Title != "Без названия" {
		t.Fatal("autosave published a draft or lost fallback title")
	}
	form.Set("id", fmt.Sprint(p.ID))
	form.Set("version", fmt.Sprint(p.Version))
	form.Set("title", "Publication")
	form.Set("slug", "publication")
	form.Set("intent", "publish")
	status(t, adminHTML(b, "POST", "/admin/pages/save", form, true, false, true), 303)
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	form.Set("intent", "autosave")
	form.Set("version", fmt.Sprint(p.Version))
	form.Set("title", "PRIVATE DRAFT")
	response = b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true)
	status(t, response, 200)
	// Repeating the old version must return a JSON conflict and leave both versions intact.
	response = b.request("POST", "/admin/pages/save", form.Encode(), "application/x-www-form-urlencoded", true)
	status(t, response, 409)
	if !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatal("autosave conflict returned an editor screen")
	}
	public, err := s.Public("publication")
	if err != nil || public.Title != "Publication" {
		t.Fatal("autosave changed the public snapshot")
	}
	home := b.request("GET", fmt.Sprintf("/?term=%d", taxonomy.ID), "", "", false)
	status(t, home, 200)
	if !strings.Contains(home.Body.String(), "Новости") || !strings.Contains(home.Body.String(), "Publication") || strings.Contains(home.Body.String(), "PRIVATE DRAFT") {
		t.Fatal("public filter leaks drafts or omits taxonomy")
	}
	editor := adminHTML(b, "GET", fmt.Sprintf("/admin/pages/%d/edit", p.ID), nil, true, false, false)
	status(t, editor, 200)
	for _, marker := range []string{`data-editor-command="underline"`, `data-editor-command="alignCenter"`, `data-editor-command="search"`, "editor-menu-popup", "Категории и теги", "schedule-form"} {
		if !strings.Contains(editor.Body.String(), marker) {
			t.Fatalf("editor missing %s", marker)
		}
	}
	body := editor.Body.String()
	order := []string{"editor-commandbar", `id="page-title"`, "editor-document", `role="toolbar"`, `id="rich-editor"`, `class="editor-settings"`, "<h2>Публикация</h2>", `data-editor-panel="Категории и теги"`}
	if strings.Count(body, `id="admin-nav-status"`) != 1 || strings.Contains(body, `class="gp-topbar"`) {
		t.Fatal("editor must have one command bar and one request status")
	}
	previous := -1
	for _, marker := range order {
		position := strings.Index(body, marker)
		if position <= previous {
			t.Fatalf("editor layout order: %s", marker)
		}
		previous = position
	}
	if strings.Count(body, `role="toolbar"`) != 1 || strings.Contains(body, "Хорошая история начинается") {
		t.Fatal("duplicated toolbar or decorative heading")
	}
	p, err = s.Page(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(2 * time.Hour).In(loc).Format("2006-01-02T15:04")
	schedule := url.Values{"version": {fmt.Sprint(p.Version)}, "publish_at": {at}}
	status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/schedule", p.ID), schedule, true, false, false), 403)
	status(t, adminHTML(b, "POST", fmt.Sprintf("/admin/pages/%d/schedule", p.ID), schedule, true, false, true), 303)
	p, err = s.Page(p.ID)
	if err != nil || p.ScheduledAt == 0 || p.ScheduledVersion != p.Version {
		t.Fatal("schedule not stored")
	}
	n, err := s.PublishDue(ctx, p.ScheduledAt)
	if err != nil || n != 1 {
		t.Fatalf("due publication %d %v", n, err)
	}
	public, err = s.Public("publication")
	if err != nil || public.Title != "PRIVATE DRAFT" {
		t.Fatal("scheduled snapshot not published")
	}
}

func TestPublicBlogPaginationUsesPublishedPosts(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Blog", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 13; i++ {
		if _, err := app.Service.Save(ctx, store.Page{Kind: "post", Title: fmt.Sprintf("Post %02d", i), Slug: fmt.Sprintf("post-%d", i), Document: document}, true); err != nil {
			t.Fatal(err)
		}
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	first := b.request("GET", "/", "", "", false)
	status(t, first, 200)
	second := b.request("GET", "/?page=2", "", "", false)
	status(t, second, 200)
	if strings.Count(first.Body.String(), `class="post-card"`)+strings.Count(first.Body.String(), `class="featured-story"`)+strings.Count(first.Body.String(), `class="spotlight-story"`) != 12 || strings.Count(second.Body.String(), `class="post-card"`) != 1 {
		t.Fatal("public pagination loses posts")
	}
	if !strings.Contains(first.Body.String(), "13 записей") {
		t.Fatal("wrong publication total")
	}
}

func TestScheduledFailureVisibleInEditor(t *testing.T) {
	s, _, b := seoApp(t)
	ctx := context.Background()
	parent, err := s.Save(ctx, store.Page{Title: "Parent", Slug: "parent", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Save(ctx, store.Page{Title: "Child", Slug: "child", ParentID: parent.ID, Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Schedule(ctx, child.ID, child.Version, 100); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeContent(ctx, parent.ID, parent.Version, "unpublish"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PublishDue(ctx, 101); err != nil {
		t.Fatal(err)
	}
	response := adminHTML(b, "GET", "/admin/pages/"+strconv.FormatInt(child.ID, 10)+"/edit", nil, true, false, false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Публикация по расписанию не выполнена.") || !strings.Contains(response.Body.String(), "Черновик сохранён.") {
		t.Fatal("editor hides publication failure")
	}
	child, err = s.Page(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Schedule(ctx, child.ID, child.Version, 200); err != nil {
		t.Fatal(err)
	}
	response = adminHTML(b, "GET", "/admin/pages/"+strconv.FormatInt(child.ID, 10)+"/edit", nil, true, false, false)
	status(t, response, 200)
	if strings.Contains(response.Body.String(), "Публикация по расписанию не выполнена.") {
		t.Fatal("old failure remains after rescheduling")
	}
}
