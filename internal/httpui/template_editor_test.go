package httpui

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestTemplateEditorCopySaveValidationConflictAndRestore(t *testing.T) {
	s, app, b := seoApp(t)
	reference := b.request("GET", "/admin/themes/reference", "", "", false)
	status(t, reference, 200)
	for _, marker := range []string{"template-reference-nav", "<table>", "&lt;!doctype html&gt;", ".Pagination.Current", "Template file editor"} {
		if !strings.Contains(reference.Body.String(), marker) {
			t.Fatal("reference missing", marker)
		}
	}
	if reference.Header().Get("Content-Disposition") != "" {
		t.Fatal("reference is still a download")
	}
	readonly := b.request("GET", templateEditorURL("minimal", "layout.html"), "", "", false)
	status(t, readonly, 200)
	for _, marker := range []string{`hx-target="#template-file-editor"`, `hx-select="#template-file-editor"`, `hx-swap="outerHTML show:none"`, `data-template-file="layout.html"`, `wrap="soft"`} {
		if !strings.Contains(readonly.Body.String(), marker) {
			t.Fatal("unstable file navigation", marker)
		}
	}

	if !strings.Contains(readonly.Body.String(), "Создать редактируемую копию") || !strings.Contains(readonly.Body.String(), " readonly") {
		t.Fatal("builtin editable")
	}
	// The editor's dynamic CSS gets a per-document nonce, retained by the full admin shell during SPA navigation.
	nonceStart := strings.Index(readonly.Body.String(), `name="style-nonce" content="`)
	if nonceStart < 0 {
		t.Fatal("missing nonce")
	}
	nonce := strings.Split(readonly.Body.String()[nonceStart+len(`name="style-nonce" content="`):], `"`)[0]
	if nonce == "" || !strings.Contains(readonly.Header().Get("Content-Security-Policy"), "'nonce-"+nonce+"'") {
		t.Fatal("CSP nonce mismatch")
	}
	status(t, adminHTML(b, "POST", "/admin/themes/editor/copy", url.Values{"theme": {"minimal"}}, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/themes/editor/copy", url.Values{"theme": {"minimal"}}, false, false, true), 303)
	id := "theme:minimal-copy"
	files, dir, err := app.templateFiles(id)
	if err != nil {
		t.Fatal(err)
	}
	original := string(files["page.html"].Data)
	if !strings.Contains(string(files["layout.html"].Data), `{{asset .ThemeAssets "public-minimal.css"}}`) {
		t.Fatal("copy uses built-in CSS")
	}
	appearance, err := s.Appearance()
	if err != nil || appearance.Theme == id {
		t.Fatal("copy auto-activated", err)
	}
	fields := url.Values{"theme": {id}, "file": {"page.html"}, "revision": {templateFileHash([]byte(original))}, "content": {original + "\n<p>Edited template</p>"}}
	status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, false, false, true), 303)
	edited, err := os.ReadFile(filepath.Join(dir, "page.html"))
	if err != nil || !strings.Contains(string(edited), "Edited template") {
		t.Fatal(err)
	}
	conflict := adminHTML(b, "POST", "/admin/themes/editor/save", fields, true, false, true)
	status(t, conflict, 409)
	if !strings.Contains(conflict.Body.String(), "Edited template") || !strings.Contains(conflict.Body.String(), "другой вкладке") {
		t.Fatal("conflict lost submitted edits")
	}
	fields.Set("revision", templateFileHash(edited))
	fields.Set("content", "{{if}} My unsaved changes")
	invalid := adminHTML(b, "POST", "/admin/themes/editor/save", fields, true, false, true)
	status(t, invalid, 400)
	if !strings.Contains(invalid.Body.String(), "My unsaved changes") {
		t.Fatal("invalid edits discarded")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "page.html"))
	if string(after) != string(edited) {
		t.Fatal("invalid file replaced live template")
	}
	fields.Set("intent", "restore")
	status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, false, false, true), 303)
	restored, _ := os.ReadFile(filepath.Join(dir, "page.html"))
	if string(restored) != original {
		t.Fatal("restore failed")
	}
	status(t, b.request("GET", templateEditorURL(id, "../../site.db"), "", "", false), 404)
	appearance.Theme = id
	if err = app.Service.SaveAppearance(context.Background(), appearance); err != nil {
		t.Fatal(err)
	}
	preview := b.request("GET", "/admin/themes/view?theme="+id, "", "", false)
	status(t, preview, 200)
	status(t, b.request("GET", "/theme-assets/minimal-copy/public-minimal.css", "", "", false), 200)
	for _, builtin := range []string{"journal", "studio"} {
		if _, err = app.copyBuiltinTemplate(builtin); err != nil {
			t.Fatal("builtin copy", builtin, err)
		}
	}
}
func TestTemplateEditorPermissionsAndSafePaths(t *testing.T) {
	_, app, b := seoApp(t)
	for _, role := range []string{"editor", "author"} {
		email := role + "@example.com"
		if _, err := app.Service.SaveUser(context.Background(), store.User{Email: email, Role: role}, "a sufficiently long password"); err != nil {
			t.Fatal(err)
		}
		other := userBrowser(t, app, email)
		for _, p := range []string{"/admin/themes/editor?theme=journal", "/admin/themes/reference"} {
			status(t, other.request("GET", p, "", "", false), 403)
		}
	}
	for _, name := range []string{"../layout.html", "assets/../../site.db", "/page.html", "assets/code.php", "seo.html", "theme.json"} {
		if editableTemplateFile(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"layout.html", "parts/nested/card.html", "assets/css/site.css", "assets/main.js"} {
		if !editableTemplateFile(name) {
			t.Fatal(name)
		}
	}
	id, err := app.copyBuiltinTemplate("minimal")
	if err != nil {
		t.Fatal(err)
	}
	_, dir, err := app.templateFiles(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(app.Service.Store.Dir, "site.db"), filepath.Join(dir, "assets", "unsafe.css")); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", templateEditorURL(id, "assets/unsafe.css"), "", "", false), 500)
}
func TestTemplateReferenceEscapesExamples(t *testing.T) {
	sections := templateReferenceSections("# Reference\n\n## Code\n\n<script>alert(1)</script>\n\n```html\n<img onerror=evil>\n```\n")
	if len(sections) != 1 || strings.Contains(sections[0].HTML, "<script>") || !strings.Contains(sections[0].HTML, "&lt;img") {
		t.Fatal(sections)
	}
}
