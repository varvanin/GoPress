package httpui

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gopress/internal/store"
)

func TestTemplateFileOperationsGuardReferencesVersionsAndRecoverDeletedCode(t *testing.T) {
	_, app, _ := seoApp(t)
	files, dir, _, err := app.templateFiles("minimal", "layout.html", false)
	if err != nil {
		t.Fatal(err)
	}
	revision := templateFileHash(files["layout.html"].Data)
	name, err := app.changeTemplateFile("minimal", "layout.html", revision, string(files["layout.html"].Data), "extra.html", "create")
	if err != nil || name != "extra.html" {
		t.Fatal(name, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	name, err = app.changeTemplateFile("minimal", name, templateFileHash(raw), string(raw), "renamed.html", "rename")
	if err != nil || name != "renamed.html" {
		t.Fatal(name, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "extra.html")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old name remains", err)
	}
	if _, err = app.changeTemplateFile("minimal", name, "stale", string(raw), "", "delete"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale deletion accepted", err)
	}
	if _, err = app.changeTemplateFile("minimal", name, templateFileHash(raw), string(raw)+"unsaved", "", "delete"); err == nil {
		t.Fatal("discarded unsaved source")
	}
	if _, err = app.Service.Save(context.Background(), store.Page{Title: "Page", Slug: "file-use", Document: document, Template: name}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = app.changeTemplateFile("minimal", name, templateFileHash(raw), string(raw), "", "delete"); err == nil {
		t.Fatal("deleted file referenced by draft")
	}
	// An unused partial can be removed, retaining its last bytes outside the live template.
	files, _, _, err = app.templateFiles("minimal", "layout.html", false)
	if err != nil {
		t.Fatal(err)
	}
	name, err = app.changeTemplateFile("minimal", "layout.html", templateFileHash(files["layout.html"].Data), string(files["layout.html"].Data), "parts/unused.html", "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.changeTemplateFile("minimal", name, templateFileHash(nil), "", "", "delete"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(app.templateHistoryPath("minimal", name) + ".deleted"); err != nil {
		t.Fatal("missing deleted-file recovery", err)
	}
	if _, err = app.changeTemplateFile("minimal", "layout.html", templateFileHash(files["layout.html"].Data), string(files["layout.html"].Data), "../escape.html", "create"); err == nil {
		t.Fatal("accepted traversal")
	}
	if !templateFileReferenced(fstest.MapFS{"parts/a.html": {Data: []byte(`{{if .Title}}{{template "parts/b" .}}{{end}}`)}}, "parts/b.html") {
		t.Fatal("conditional reference missed")
	}
}
func TestTemplateExportAndResetKeepLocalRecoveryAndActiveSelection(t *testing.T) {
	s, app, b := seoApp(t)
	before, _ := s.Appearance()
	files, dir, _, err := app.templateFiles("minimal", "layout.html", false)
	if err != nil {
		t.Fatal(err)
	}
	original := string(files["layout.html"].Data)
	if err = app.saveTemplateFile("minimal", "layout.html", templateFileHash([]byte(original)), original+"\n<!-- Changed -->", false); err != nil {
		t.Fatal(err)
	}
	exported := b.request("GET", "/admin/themes/editor/export?theme=minimal", "", "", false)
	status(t, exported, 200)
	info, err := app.installTheme(strings.NewReader(exported.Body.String()))
	if err != nil || info.ID != "theme:minimal-custom" {
		t.Fatal(info, err)
	}
	review := b.request("GET", "/admin/themes/editor/reset?theme=minimal", "", "", false)
	status(t, review, 200)
	current, _, _, err := app.templateFiles("minimal", "", false)
	if err != nil {
		t.Fatal(err)
	}
	revision := templateTreeHash(current)
	status(t, adminHTML(b, "POST", "/admin/themes/editor/reset", url.Values{"theme": {"minimal"}, "revision": {"stale"}}, false, false, true), 409)
	status(t, adminHTML(b, "POST", "/admin/themes/editor/reset", url.Values{"theme": {"minimal"}, "revision": {revision}}, false, false, true), 303)
	if _, err = os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("local override remains", err)
	}
	restored, _, _, err := app.templateFiles("minimal", "layout.html", false)
	if err != nil || string(restored["layout.html"].Data) != original {
		t.Fatal("not reset to embedded originals", err)
	}
	backup := filepath.Join(filepath.Dir(app.templateHistoryPath("minimal", "layout.html")), "before-reset", "layout.html")
	raw, err := os.ReadFile(backup)
	if err != nil || !strings.Contains(string(raw), "Changed") {
		t.Fatal("reset lost local recovery", err)
	}
	appearance, err := s.Appearance()
	if err != nil || appearance.Theme != before.Theme {
		t.Fatal("reset changed activation", appearance, err)
	}
}
func TestTemplatePreviewIsPrivateBoundedAndNeverSavesSource(t *testing.T) {
	_, app, b := seoApp(t)
	files, _, _, err := app.templateFiles("minimal", "page.html", false)
	if err != nil {
		t.Fatal(err)
	}
	original := string(files["page.html"].Data)
	values := url.Values{"theme": {"minimal"}, "file": {"page.html"}, "revision": {templateFileHash([]byte(original))}, "content": {original + "<p>UNSAVED PRIVATE PREVIEW</p>"}}
	status(t, adminHTML(b, "POST", "/admin/themes/editor/preview", values, false, false, false), 403)
	preview := adminHTML(b, "POST", "/admin/themes/editor/preview", values, false, false, true)
	status(t, preview, 303)
	location := preview.Header().Get("Location")
	response := b.request("GET", location, "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "UNSAVED PRIVATE PREVIEW") || !strings.Contains(response.Body.String(), location+"/assets/") {
		t.Fatal("preview missing source or private asset base", response.Body.String())
	}
	unchanged, _, _, err := app.templateFiles("minimal", "page.html", false)
	if err != nil || string(unchanged["page.html"].Data) != original {
		t.Fatal("preview saved source", err)
	}
	token := strings.TrimPrefix(location, "/admin/themes/editor/previews/")
	if _, ok := app.getTemplatePreview(token, 999); ok {
		t.Fatal("another user can access preview")
	}
	css := b.request("GET", location+"/assets/public-minimal.css", "", "", false)
	status(t, css, 200)
	if _, ok := app.getTemplatePreview(token, 1); !ok {
		t.Fatal("owner lost preview")
	}
	app.templatePreviewMu.Lock()
	entry := app.templatePreviews[token]
	entry.Expires = time.Now().Add(-time.Second)
	app.templatePreviews[token] = entry
	app.templatePreviewMu.Unlock()
	status(t, b.request("GET", location, "", "", false), 404)
	for i := 0; i < 25; i++ {
		app.putTemplatePreview(entry)
	}
	if len(app.templatePreviews) > 16 {
		t.Fatal("preview state not bounded")
	}
}
