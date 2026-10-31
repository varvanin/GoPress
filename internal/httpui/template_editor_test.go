package httpui

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

func TestTemplateEditorCopySaveValidationConflictAndRestore(t *testing.T) {
	s, app, b := seoApp(t)
	reference := b.request("GET", "/admin/themes/reference", "", "", false)
	status(t, reference, 200)
	for _, marker := range []string{"template-reference-nav", "<table>", "&lt;!doctype html&gt;", ".Pagination.Current", "Редактор файлов шаблона"} {
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

	if strings.Contains(readonly.Body.String(), "Создать редактируемую копию") || strings.Contains(readonly.Body.String(), " readonly") || !strings.Contains(readonly.Body.String(), "Сохранить файл") {
		t.Fatal("builtin editor must allow direct editing")
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
	files, dir, _, err := app.templateFiles(id, "", false)
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
	_, dir, _, err := app.templateFiles(id, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(app.Service.Store.Dir, "site.db"), filepath.Join(dir, "assets", "unsafe.css")); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", templateEditorURL(id, "assets/unsafe.css"), "", "", false), 500)
}
func TestTemplateReferenceEscapesExamples(t *testing.T) {
	sections := templateReferenceSections(i18n.WithLanguage(context.Background(), "en"), "# Reference\n\n## Code\n\n<script>alert(1)</script>\n\n```html\n<img onerror=evil>\n```\n")
	if len(sections) != 1 || strings.Contains(sections[0].HTML, "<script>") || !strings.Contains(sections[0].HTML, "&lt;img") {
		t.Fatal(sections)
	}
}

func TestTemplateReferenceRussianCopyPreservesExamplesAndAnchors(t *testing.T) {
	raw, err := resources.ReadFile("starter/TEMPLATE_REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	en := templateReferenceSections(i18n.WithLanguage(context.Background(), "en"), string(raw))
	ru := templateReferenceSections(i18n.WithLanguage(context.Background(), "ru"), string(raw))
	if len(en) != len(ru) || ru[1].Title != "Выбор файла" {
		t.Fatal("missing translated sections")
	}
	for i, section := range ru {
		if section.ID != en[i].ID {
			t.Fatal("translated anchor changed")
		}
		for _, part := range strings.Split(en[i].HTML, "<pre><code>")[1:] {
			code := strings.SplitN(part, "</code></pre>", 2)[0]
			if !strings.Contains(section.HTML, "<pre><code>"+code+"</code></pre>") {
				t.Fatal("translated code example")
			}
		}
	}
	if !strings.Contains(ru[1].HTML, "Порядок выбора") || strings.Contains(ru[1].HTML, "Selection order") {
		t.Fatal("untranslated table")
	}
	for source, translation := range templateReferenceRussian {
		sourceCode := strings.Split(source, "`")
		translatedCode := strings.Split(translation, "`")
		if len(sourceCode) != len(translatedCode) {
			t.Fatalf("changed inline code: %s", source)
		}
		for i := 1; i < len(sourceCode); i += 2 {
			if sourceCode[i] != translatedCode[i] {
				t.Fatalf("changed identifier: %s", source)
			}
		}
	}
}

func TestTemplateSelectionReadsOnlyTheSelectedContents(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	root := filepath.Join(s.Dir, "theme")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"layout.html": []byte("layout"), "page.html": []byte("page"), "assets/image.webp": make([]byte, 4<<20)} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	selected, dir, _, err := app.templateFiles("custom", "page.html", false)
	if err != nil || dir != root || string(selected["page.html"].Data) != "page" {
		t.Fatal(dir, err)
	}
	if len(selected["assets/image.webp"].Data) != 0 || len(selected["layout.html"].Data) != 0 {
		t.Fatal("selection reads unrelated payloads")
	}
	full, _, _, err := app.templateFiles("custom", "", false)
	if err != nil || len(full["assets/image.webp"].Data) != 4<<20 {
		t.Fatal("copy/validation no longer receives the full package", err)
	}
}

func TestBuiltinTemplatesDirectSaveAssetsRestoreAndDelete(t *testing.T) {
	s, app, b := seoApp(t)
	for _, id := range []string{"journal", "minimal", "studio"} {
		files, dir, _, err := app.templateFiles(id, "", false)
		if err != nil {
			t.Fatal(err)
		}
		original := string(files["layout.html"].Data)
		fields := url.Values{"theme": {id}, "file": {"layout.html"}, "revision": {templateFileHash([]byte(original))}, "content": {strings.Replace(original, "</body>", "<p>Direct edit</p></body>", 1)}}
		if _, err = os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("GET created template files", err)
		}
		fields.Set("content", "{{if}} unsaved")
		status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, true, false, true), 400)
		if _, err = os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("invalid save installed files")
		}
		fields.Set("content", strings.Replace(original, "</body>", "<p>Direct edit</p></body>", 1))
		status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, false, false, false), 403)
		status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, false, false, true), 303)
		status(t, adminHTML(b, "POST", "/admin/themes/editor/save", fields, true, false, true), 409)
		appearance, err := s.Appearance()
		if err != nil {
			t.Fatal(err)
		}
		appearance.Theme = id
		if err = app.Service.SaveAppearance(context.Background(), appearance); err != nil {
			t.Fatal(err)
		}
		page := b.request("GET", "/", "", "", false)
		status(t, page, 200)
		if !strings.Contains(page.Body.String(), "Direct edit") || !strings.Contains(page.Body.String(), "/builtin-assets/"+id+"/") {
			t.Fatal("direct changes not public", id, page.Body.String())
		}
		css := "public.css"
		if id == "minimal" {
			css = "public-minimal.css"
		}
		if id == "studio" {
			css = "public-studio.css"
		}
		cssOriginal := string(files["assets/"+css].Data)
		if err = app.saveTemplateFile(id, "assets/"+css, templateFileHash([]byte(cssOriginal)), cssOriginal+"\n/* direct css */", false); err != nil {
			t.Fatal(err)
		}
		asset := b.request("GET", "/builtin-assets/"+id+"/"+css, "", "", false)
		status(t, asset, 200)
		if !strings.Contains(asset.Body.String(), "direct css") {
			t.Fatal("CSS not saved")
		}
		edited, err := os.ReadFile(filepath.Join(dir, "layout.html"))
		if err != nil {
			t.Fatal(err)
		}
		if err = app.saveTemplateFile(id, "layout.html", templateFileHash(edited), "", true); err != nil {
			t.Fatal(err)
		}
		restored, _ := os.ReadFile(filepath.Join(dir, "layout.html"))
		if string(restored) != original {
			t.Fatal("restore changed builtin")
		}
		status(t, adminHTML(b, "POST", "/admin/themes/delete", url.Values{"theme": {id}}, false, false, true), 400)
	}
	// Studio is active; deleting another builtin removes it without touching content or packages.
	status(t, adminHTML(b, "POST", "/admin/themes/delete", url.Values{"theme": {"minimal"}}, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/themes/delete", url.Values{"theme": {"minimal"}}, false, false, true), 303)
	themes, err := app.themes()
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range themes {
		if theme.ID == "minimal" {
			t.Fatal("deleted builtin still listed")
		}
	}
	if _, _, _, err = app.themeSource("minimal"); err == nil {
		t.Fatal("deleted builtin resolves")
	}
	status(t, b.request("GET", templateEditorURL("minimal", "layout.html"), "", "", false), 404)
	if _, err = os.Stat(app.builtinTemplatePath("minimal")); !os.IsNotExist(err) {
		t.Fatal("deleted local files remain")
	}
	status(t, b.request("GET", "/builtin-assets/minimal/public-minimal.css", "", "", false), 404)
	status(t, b.request("GET", "/builtin-assets/not-a-builtin/public.css", "", "", false), 404)
	// Markers live under theme/ and survive a new App, like backups and updates.
	fresh := &App{Service: app.Service}
	if _, _, _, err = fresh.themeSource("minimal"); err == nil {
		t.Fatal("deleted builtin reappeared")
	}
}
