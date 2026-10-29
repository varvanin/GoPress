package httpui

import (
	"context"
	"encoding/json"
	"errors"
	"gopress/internal/backup"
	"gopress/internal/service"
	"gopress/internal/store"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCollectionsLifecycleSecurityAndBackup(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	fixture, err := os.Open("../../scripts/fixtures/smoke.png")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	image, err := app.Service.Upload(fixture)
	if err != nil {
		t.Fatal(err)
	}
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	if _, err = app.Service.SaveUser(ctx, store.User{Email: "collection-editor@example.test", Role: "editor"}, "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	editor := userBrowser(t, app, "collection-editor@example.test")
	for _, kind := range []string{"gallery", "testimonials"} {
		root := "/admin/modules/" + kind
		status(t, anon.request("GET", root, "", "", false), 401)
		status(t, editor.request("GET", root, "", "", false), 403)
		fields := url.Values{"id": {"0"}, "version": {"0"}, "name": {"Collection <script>"}, "layout": {"grid"}, "image": {image}, "caption": {"Caption <script>"}, "person": {"Alice <script>"}, "company": {"Example"}, "text": {"An excellent result. <script>alert(1)</script>"}}
		status(t, adminHTML(b, "POST", root+"/save", fields, false, false, false), 403)
		status(t, adminHTML(b, "POST", root+"/save", fields, false, false, true), 303)
		groups, err := s.Collections(kind)
		if err != nil || len(groups) != 1 {
			t.Fatal(groups, err)
		}
		group, err := s.Collection(kind, groups[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		status(t, adminHTML(b, "GET", collectionPath(kind, group.ID), nil, false, false, false), 200)
		picker := adminHTML(b, "GET", "/admin/modules/media/choices?q=", nil, true, false, false)
		status(t, picker, 200)
		if !strings.Contains(picker.Body.String(), `data-collection-choice="`+image+`"`) {
			t.Fatal("media choice missing")
		}
		if err = app.Service.DeleteMedia(ctx, image); !errors.Is(err, store.ErrMediaUsed) {
			t.Fatal("disabled collection did not protect media", err)
		}
		document, _ := json.Marshal(service.Node{Type: "doc", Content: []service.Node{{Type: "paragraph", Content: []service.Node{{Type: "text", Text: collectionCode(group)}}}}})
		page, err := app.Service.Save(ctx, store.Page{Title: kind, Slug: kind + "-example", Document: string(document)}, true)
		if err != nil {
			t.Fatal(err)
		}
		read := func() string {
			w := anon.request("GET", "/"+page.Slug, "", "", false)
			status(t, w, 200)
			return w.Body.String()
		}
		if body := read(); strings.Contains(body, "gp-collection") || strings.Contains(body, "["+kind+" id=") {
			t.Fatal("disabled collection leaked")
		}
		status(t, adminHTML(b, "POST", root+"/toggle", url.Values{"enabled": {"true"}}, false, false, true), 303)
		body := read()
		if !strings.Contains(body, "gp-"+kind) || !strings.Contains(body, image) || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
			t.Fatal("public collection missing or unsafe")
		}
		nav := adminHTML(b, "GET", "/admin/pages", nil, true, false, false)
		if !strings.Contains(nav.Body.String(), `href="`+root+`"`) {
			t.Fatal("enabled collection missing in sidebar")
		}
		fields.Set("id", strconv.FormatInt(group.ID, 10))
		fields.Set("version", "0")
		fields.Set("name", "Submitted conflict name")
		conflict := adminHTML(b, "POST", root+"/save", fields, false, false, true)
		status(t, conflict, 400)
		fields.Set("version", "99")
		conflict = adminHTML(b, "POST", root+"/save", fields, false, false, true)
		status(t, conflict, 409)
		if !strings.Contains(conflict.Body.String(), "Submitted conflict name") {
			t.Fatal("conflict lost fields")
		}
		fields.Set("version", "1")
		fields.Set("image", "../unsafe.svg")
		status(t, adminHTML(b, "POST", root+"/save", fields, false, false, true), 400)
		current, err := s.Collection(kind, group.ID)
		if err != nil || current.Version != 1 {
			t.Fatal("invalid save changed collection", err)
		}
	}
	snapshot := filepath.Join(t.TempDir(), "backup")
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = backup.Create(s.Dir, snapshot); err != nil {
		t.Fatal(err)
	}
	if err = backup.Verify(snapshot); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if err = backup.Restore(snapshot, dest); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	for _, kind := range []string{"gallery", "testimonials"} {
		enabled, err := restored.ModuleEnabled(kind)
		if err != nil || !enabled {
			t.Fatal("backup lost module state", err)
		}
		groups, err := restored.Collections(kind)
		if err != nil || len(groups) != 1 {
			t.Fatal("backup lost collections", err)
		}
		group, err := restored.Collection(kind, groups[0].ID)
		if err != nil || group.Items[0].Image != image {
			t.Fatal("backup lost media reference", err)
		}
		if err = restored.DeleteCollection(ctx, kind, group.ID, 99); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale delete accepted")
		}
		if err = restored.DeleteCollection(ctx, kind, group.ID, group.Version); err != nil {
			t.Fatal(err)
		}
	}
	if used, err := restored.MediaUsed(image); err != nil || used {
		t.Fatal("collection deletion did not release media", err)
	}
}
func TestCollectionValidationAndOrdering(t *testing.T) {
	s, app, _ := seoApp(t)
	ctx := context.Background()
	file, err := os.Open("../../scripts/fixtures/smoke.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := app.Service.Upload(file)
	if err != nil {
		t.Fatal(err)
	}
	c := store.Collection{Kind: "gallery", Name: "Gallery", Layout: "grid", Items: []store.CollectionItem{{Image: image, Caption: "First"}, {Image: image, Caption: "Second"}}}
	id, err := app.Service.SaveCollection(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Collection("gallery", id)
	if err != nil {
		t.Fatal(err)
	}
	c.Items[0], c.Items[1] = c.Items[1], c.Items[0]
	if _, err = app.Service.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Collection("gallery", id)
	if err != nil || saved.Items[0].Caption != "Second" {
		t.Fatal("order not saved", err)
	}
	if _, err = app.Service.SaveCollection(store.WithActor(ctx, store.User{ID: 1, Role: "editor"}), c); !errors.Is(err, service.ErrForbidden) {
		t.Fatal("non-admin save accepted")
	}
	for _, bad := range []store.Collection{{Kind: "gallery", Name: "G", Layout: "grid", Items: []store.CollectionItem{{}}}, {Kind: "testimonials", Name: "T", Layout: "grid", Items: []store.CollectionItem{{Name: "Alice"}}}, {Kind: "gallery", Name: "G", Layout: "list", Items: c.Items}, {Kind: "gallery", Name: "G", Layout: "grid", Items: make([]store.CollectionItem, 51)}} {
		if _, err = app.Service.SaveCollection(ctx, bad); !errors.Is(err, service.ErrInvalid) {
			t.Fatal("invalid collection accepted", err)
		}
	}
}

func TestTestimonialsOptionalPhotoAndList(t *testing.T) {
	s, app, _ := seoApp(t)
	ctx := context.Background()
	item := store.Collection{Kind: "testimonials", Layout: "list", Name: "Client voices", Items: []store.CollectionItem{{Name: "Alice", Company: "Studio", Text: "Very helpful."}}}
	id, err := app.Service.SaveCollection(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetModuleEnabled(ctx, "testimonials", true); err != nil {
		t.Fatal(err)
	}
	item, err = s.Collection("testimonials", id)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := json.Marshal(service.Node{Type: "doc", Content: []service.Node{{Type: "paragraph", Content: []service.Node{{Type: "text", Text: collectionCode(item)}}}}})
	page, err := app.Service.Save(ctx, store.Page{Title: "Voices", Slug: "voices", Document: string(document)}, true)
	if err != nil {
		t.Fatal(err)
	}
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	w := anon.request("GET", "/"+page.Slug, "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), `data-layout="list"`) || !strings.Contains(w.Body.String(), "Very helpful.") || strings.Contains(w.Body.String(), `src="/uploads/"`) {
		t.Fatal("optional photo/list output incorrect")
	}
}
