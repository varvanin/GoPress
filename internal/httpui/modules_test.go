package httpui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/service"
	"gopress/internal/store"
)

func TestUploadedModuleLifecycleAndBackup(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("GET", "/admin/modules/upload", "", "", false), 401)
	for _, role := range []string{"editor", "author"} {
		email := role + "@example.com"
		if _, err := app.Service.SaveUser(ctx, store.User{Email: email, Role: role}, "a sufficiently long password"); err != nil {
			t.Fatal(err)
		}
		other := userBrowser(t, app, email)
		status(t, other.request("GET", "/admin/modules/upload", "", "", false), 403)
		status(t, other.request("GET", "/admin/modules/starter.zip", "", "", false), 403)
	}
	starter := b.request("GET", "/admin/modules/starter.zip", "", "", false)
	status(t, starter, 200)
	upload := func(csrf bool) int {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if csrf {
			if err := w.WriteField("csrf", b.cookies["csrf"].Value); err != nil {
				t.Fatal(err)
			}
		}
		f, err := w.CreateFormFile("module_zip", "module.zip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(starter.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		return b.request("POST", "/admin/modules/upload", buf.String(), w.FormDataContentType(), false).Code
	}
	if code := upload(false); code != 403 {
		t.Fatal(code)
	}
	if code := upload(true); code != 303 {
		t.Fatal(code)
	}
	if code := upload(true); code != 400 {
		t.Fatal(code)
	}
	p, err := s.ModulePackage("contact-card")
	if err != nil || p.Enabled {
		t.Fatal(p, err)
	}
	catalog := b.request("GET", "/admin/modules", "", "", false)
	status(t, catalog, 200)
	for _, marker := range []string{"theme-card", "module-catalog", "contact-card", "/admin/modules/upload", "/admin/modules/starter.zip", "data-module-id=\"faq\""} {
		if !strings.Contains(catalog.Body.String(), marker) {
			t.Fatal("catalog missing", marker)
		}
	}
	status(t, b.request("GET", "/admin/modules/package/contact-card/edit", "", "", false), 200)
	status(t, anon.request("GET", "/module-assets/contact-card/screenshot", "", "", false), 200)
	status(t, anon.request("GET", "/module-assets/contact-card/style.css", "", "", false), 404)
	doc := service.Node{Type: "doc", Content: []service.Node{{Type: "paragraph", Content: []service.Node{{Type: "text", Text: moduleCode(p.ID)}}}}}
	raw, _ := json.Marshal(doc)
	page, err := app.Service.Save(ctx, store.Page{Title: "Module page", Slug: "module-page", Document: string(raw)}, true)
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		r := anon.request("GET", "/"+page.Slug, "", "", false)
		status(t, r, 200)
		return r.Body.String()
	}
	if body := read(); strings.Contains(body, "gp-module-contact-card") || strings.Contains(body, "[module id=") {
		t.Fatal("disabled block/code visible")
	}
	fields := url.Values{"id": {p.ID}, "version": {"1"}, "setting_title": {"<script>unsafe</script>"}, "setting_text": {"Personal answer"}, "setting_address": {"Warsaw"}, "setting_link": {"https://example.org"}, "setting_button": {"Contact"}}
	status(t, adminHTML(b, "POST", "/admin/modules/package/save", fields, false, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/modules/package/save", fields, false, false, true), 303)
	status(t, adminHTML(b, "POST", "/admin/modules/package/save", fields, false, false, true), 409)
	toggle := url.Values{"id": {p.ID}, "version": {"2"}, "enabled": {"true"}}
	status(t, adminHTML(b, "POST", "/admin/modules/package/toggle", toggle, false, false, true), 303)
	body := read()
	if !strings.Contains(body, "gp-module-contact-card") || !strings.Contains(body, "Personal answer") || strings.Contains(body, "<script>unsafe</script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("unsafe or missing module", body)
	}
	status(t, anon.request("GET", "/module-assets/contact-card/style.css", "", "", false), 200)
	for _, id := range []string{"journal", "minimal", "studio"} {
		appearance, err := s.Appearance()
		if err != nil {
			t.Fatal(err)
		}
		appearance.Theme = id
		if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(read(), "gp-module-contact-card") {
			t.Fatal("module missing in template", id)
		}
	}

	status(t, adminHTML(b, "POST", "/admin/modules/package/delete", url.Values{"id": {p.ID}, "version": {"3"}}, false, false, true), 409)
	dest := filepath.Join(t.TempDir(), "snapshot")
	if err = backup.CreateFromStore(s, dest); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restored.ModulePackage(p.ID)
	restored.Close()
	if err != nil || !saved.Enabled || !strings.Contains(saved.Settings, "Personal answer") {
		t.Fatal("backup lost module", err)
	}
	toggle.Set("version", "3")
	toggle.Set("enabled", "false")
	status(t, adminHTML(b, "POST", "/admin/modules/package/toggle", toggle, false, false, true), 303)
	if strings.Contains(read(), "gp-module-contact-card") {
		t.Fatal("disable failed")
	}
	status(t, adminHTML(b, "POST", "/admin/modules/package/delete", url.Values{"id": {p.ID}, "version": {"4"}}, false, false, true), 303)
	if _, err = s.ModulePackage(p.ID); err == nil {
		t.Fatal("not deleted")
	}
	if enabled, err := s.FAQEnabled(); err != nil || enabled {
		t.Fatal("builtin changed")
	}
}
func starterModuleMap(t *testing.T) map[string]string {
	t.Helper()
	data, err := moduleStarter()
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		result[f.Name] = string(raw)
	}
	return result
}
func TestModulePackageValidation(t *testing.T) {
	_, app, _ := seoApp(t)
	for _, tc := range []struct {
		name string
		edit func(map[string]string)
	}{
		{"traversal", func(f map[string]string) { f["../escape"] = "x" }},
		{"script file", func(f map[string]string) { f["script.js"] = "alert(1)" }},
		{"syntax", func(f map[string]string) { f["template.html"] = "Line one\n{{if}}" }},
		{"missing data", func(f map[string]string) { f["template.html"] = "{{.Settings.unknown}}" }},
		{"hidden missing data", func(f map[string]string) {
			f["template.html"] = `{{if eq .Settings.title "unreachable"}}{{.Settings.unknown}}{{end}}`
		}},
		{"recursive", func(f map[string]string) {
			f["template.html"] = `{{define "loop"}}{{template "loop" .}}{{end}}{{template "loop" .}}`
		}},
		{"huge format", func(f map[string]string) { f["template.html"] = `{{printf "%100000000s" "x"}}` }},
		{"integer loop", func(f map[string]string) { f["template.html"] = `{{range 1000000000}}{{end}}` }},
		{"nested loop", func(f map[string]string) {
			f["template.html"] = `{{range .Settings}}{{range $.Settings}}{{end}}{{end}}`
		}},
		{"reserved", func(f map[string]string) {
			f["module.json"] = strings.Replace(f["module.json"], "contact-card", "faq", 1)
		}},
		{"invalid screenshot", func(f map[string]string) { f["screenshot.png"] = "bad" }},
		{"oversized template", func(f map[string]string) { f["template.html"] = strings.Repeat("x", 129<<10) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := starterModuleMap(t)
			tc.edit(files)
			_, err := app.installModule(context.Background(), bytes.NewReader(themeZip(t, files)))
			if err == nil {
				t.Fatal("invalid package accepted")
			}
			if tc.name == "syntax" && !strings.Contains(err.Error(), "template.html:2") {
				t.Fatal("missing file and line", err)
			}
		})
	}
	files := starterModuleMap(t)
	wrapped := map[string]string{}
	for k, v := range files {
		wrapped["contact/"+k] = v
	}
	if _, err := app.installModule(context.Background(), bytes.NewReader(themeZip(t, wrapped))); err != nil {
		t.Fatal("wrapped module", err)
	}
}
func TestModuleSettingsAndTemplateLimits(t *testing.T) {
	info := ModuleInfo{Fields: []ModuleField{{Key: "link", Type: "url", Required: true}}}
	for _, value := range []string{"", "javascript:alert(1)", "/relative"} {
		if _, err := moduleSettings(info, url.Values{"link": {value}}); err == nil {
			t.Fatal(value)
		}
	}
	p := store.ModulePackage{ID: "test", Settings: `{"a":"<unsafe>"}`, Template: `{{range $key,$value := .Settings}}{{$key}}={{$value}}{{end}}`}
	out, err := renderModule(p)
	if err != nil || !strings.Contains(out, "&lt;unsafe&gt;") {
		t.Fatal(out, err)
	}
	p.Template = strings.Repeat("x", 257<<10)
	if _, err = renderModule(p); err == nil {
		t.Fatal("oversized output accepted")
	}
	if got := service.Excerpt(fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, moduleCode("test")), 160); got != "" {
		t.Fatal("shortcode in excerpt", got)
	}
}
