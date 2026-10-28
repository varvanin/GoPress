package httpui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestMediaAndMenusHTTP(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Site", BaseURL: "http://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("csrf", b.csrf); err != nil {
		t.Fatal(err)
	}
	f, err := w.CreateFormFile("file", "Обложка.png")
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("POST", "/admin/media/upload", body.String(), w.FormDataContentType(), false), 303)
	files, total, err := s.ListMedia("Обложка", 1)
	if err != nil || total != 1 {
		t.Fatalf("%+v %v", files, err)
	}
	library := adminHTML(b, "GET", "/admin/media", nil, true, false, false)
	status(t, library, 200)
	if !strings.Contains(library.Body.String(), "Обложка.png") || strings.Contains(library.Body.String(), "<!DOCTYPE") {
		t.Fatal("library fragment missing")
	}
	status(t, b.request("GET", "/admin/media", "", "", false), 200)
	picker := adminHTML(b, "GET", "/admin/media/choices?q=notfound&cover="+files[0].Name, nil, true, false, false)
	status(t, picker, 200)
	if !strings.Contains(picker.Body.String(), files[0].Name) || !strings.Contains(picker.Body.String(), "checked") {
		t.Fatal("picker lost selection")
	}
	fields := url.Values{"position": {"header"}, "version": {"0"}, "label": {"Company", "Child"}, "url": {"/about", "/child"}, "depth": {"0", "1"}}
	status(t, adminHTML(b, "POST", "/admin/menus", fields, true, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/menus", fields, true, false, true), 303)
	status(t, adminHTML(b, "POST", "/admin/menus", fields, true, false, true), 409)
	public := b.request("GET", "/", "", "", false)
	status(t, public, 200)
	if !strings.Contains(public.Body.String(), `href="/about">Company`) || !strings.Contains(public.Body.String(), `href="/child">Child`) {
		t.Fatal("public menu missing")
	}
	fields.Set("version", "1")
	fields["url"][0] = "javascript:alert(1)"
	status(t, adminHTML(b, "POST", "/admin/menus", fields, true, false, true), 400)
	status(t, adminHTML(b, "POST", "/admin/media/delete", url.Values{"name": {files[0].Name}}, true, false, true), 303)
}

func TestMediaInspectorAndMenuPositions(t *testing.T) {
	s, app, b := seoApp(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	name, err := app.Service.Upload(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.Service.Save(context.Background(), store.Page{Title: "About", Slug: "about", Cover: name, Document: document}, true); err != nil {
		t.Fatal(err)
	}
	library := adminHTML(b, "GET", "/admin/media?file="+name, nil, true, false, false)
	status(t, library, 200)
	body := library.Body.String()
	if strings.Count(body, `action="/admin/media/details"`) != 1 || !strings.Contains(body, `class="library-inspector"`) || strings.Contains(body, `action="/admin/media/delete"`) {
		t.Fatal("expected one inspector with protected deletion")
	}
	for _, marker := range []string{`id="media-inspector"`, `hx-target="#media-inspector"`, `hx-select="#media-inspector"`, `hx-swap="outerHTML show:none"`, `hx-push-url="true"`, `data-media-name="` + name + `"`} {
		if !strings.Contains(body, marker) {
			t.Fatalf("media selection must preserve the workspace: missing %s", marker)
		}
	}
	details, err := s.MediaFile(name)
	if err != nil {
		t.Fatal(err)
	}
	response := adminHTML(b, "POST", "/admin/media/details", url.Values{"revision": {details.Revision}, "name": {name}, "title": {"Cover"}, "alt": {"Landscape"}, "caption": {"Caption"}}, true, false, true)
	status(t, response, 303)
	if !strings.Contains(response.Header().Get("Location"), "file="+name) {
		t.Fatal("metadata save lost selected file")
	}
	used, err := s.MediaUsed(name)
	if err != nil || !used {
		t.Fatal(used, err)
	}
	menus := adminHTML(b, "GET", "/admin/menus?position=footer", nil, true, false, false)
	status(t, menus, 200)
	body = menus.Body.String()
	if strings.Count(body, `id="site-menu-form"`) != 1 || !strings.Contains(body, `name="position" value="footer"`) || !strings.Contains(body, `data-templ-value="/about" data-templ-label="About"`) {
		t.Fatal("menu position/page picker missing")
	}

	if strings.Count(body, `<template data-menu-template>`) != 1 || !strings.Contains(body, `<template data-menu-template><div class="menu-row"`) || strings.Index(body, `data-templ-items`) > strings.Index(body, `data-menu-template`) {
		t.Fatal("menu rows must have a dedicated template after the select templates")
	}
	response = adminHTML(b, "POST", "/admin/menus", url.Values{"position": {"footer"}, "version": {"0"}, "label": {"Contact"}, "url": {"/contact"}, "depth": {"0"}}, true, false, true)
	status(t, response, 303)
	if !strings.Contains(response.Header().Get("Location"), "position=footer") {
		t.Fatal("save lost menu position")
	}
	status(t, adminHTML(b, "GET", "/admin/menus?position=invalid", nil, true, false, false), 400)
}

func TestMediaConflictPreservesPostedFieldsAndFileFacts(t *testing.T) {
	s, app, b := seoApp(t)
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	name, err := app.Service.Upload(bytes.NewReader(imageData.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.MediaFile(name)
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"name": {name}, "revision": {original.Revision}, "title": {"First"}, "alt": {"Landscape"}, "caption": {"Caption"}, "return_q": {"First"}, "return_page": {"1"}}
	status(t, adminHTML(b, "POST", "/admin/media/details", fields, true, false, true), 303)
	fields.Set("title", "Unsaved second editor")
	response := adminHTML(b, "POST", "/admin/media/details", fields, true, false, true)
	status(t, response, 409)
	for _, marker := range []string{"Unsaved second editor", `value="First"`, mediaSize(original.Size), "другой вкладке", `value="` + original.Revision + `"`} {
		if !strings.Contains(response.Body.String(), marker) {
			t.Fatal("conflict lost editor state", marker)
		}
	}
	current, err := s.MediaFile(name)
	if err != nil || current.Title != "First" {
		t.Fatal("stale edit overwritten", err)
	}
	fields.Set("revision", current.Revision)
	fields.Set("title", strings.Repeat("x", 201))
	status(t, adminHTML(b, "POST", "/admin/media/details", fields, true, false, true), 400)
}
