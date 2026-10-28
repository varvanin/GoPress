package httpui

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/backup"
)

func themeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func themeFiles() map[string]string {
	return map[string]string{
		"screenshot.svg":   `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800"><rect width="1280" height="800" fill="white"/></svg>`,
		"theme.json":       `{"id":"test-theme","name":"Моя тема","version":"1.2","author":"Автор","description":"Тестовая тема"}`,
		"layout.html":      `{{define "layout"}}<!doctype html><html><head>{{template "seo" .}}<link rel="stylesheet" href="{{.ThemeAssets}}/style.css"></head><body><h1>{{.Site.Title}}</h1>{{template "body" .}}</body></html>{{end}}`,
		"home.html":        `{{define "body"}}<h2>MY CUSTOM HOME</h2>{{range .Posts}}<a href="/{{.Slug}}">{{.Title}}</a>{{end}}{{end}}`,
		"page.html":        `{{define "body"}}<h2>{{.Title}}</h2>{{.Body}}{{end}}`,
		"404.html":         `{{define "body"}}CUSTOM NOT FOUND{{end}}`,
		"assets/style.css": "body{background:#fff;color:#111}",
	}
}
func TestThemeInstallationAndLifecycle(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	settings, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	settings.Theme = "journal"
	settings.HomeTitle = "Keep home"
	settings.SEO.Title = "Keep SEO"
	if err = app.Service.SaveAppearance(ctx, settings); err != nil {
		t.Fatal(err)
	}
	starter := b.request("GET", "/admin/themes/starter.zip", "", "", false)
	status(t, starter, 200)
	if !strings.Contains(starter.Header().Get("Content-Disposition"), ".zip") {
		t.Fatal("starter is not a download")
	}
	reader, err := zip.NewReader(bytes.NewReader(starter.Body.Bytes()), int64(starter.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	filesInArchive := map[string]bool{}
	for _, file := range reader.File {
		filesInArchive[file.Name] = true
	}
	for _, name := range []string{"layout.html", "header.html", "footer.html", "home.html", "single.html", "archive.html", "search.html", "404.html", "page.html", "parts/post-card.html", "parts/pagination.html", "README.md", "TEMPLATE_REFERENCE.md"} {
		if !filesInArchive[name] {
			t.Fatal("starter missing", name)
		}
	}
	reference := b.request("GET", "/admin/themes/reference.md", "", "", false)
	status(t, reference, 200)
	if !strings.Contains(reference.Body.String(), "Pagination.NextURL") {
		t.Fatal("incomplete reference")
	}

	info, err := app.installTheme(bytes.NewReader(starter.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "theme:my-journal" {
		t.Fatal(info)
	}
	// Installing packages must not hijack the legacy theme selection.
	_, _, resolved, err := app.themeSource("")
	if err != nil || resolved != "journal" {
		t.Fatal(resolved, err)
	}
	upload := func(data []byte, csrf bool) int {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if csrf {
			if err = writer.WriteField("csrf", b.cookies["csrf"].Value); err != nil {
				t.Fatal(err)
			}
		}
		file, e := writer.CreateFormFile("theme_zip", "theme.zip")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = file.Write(data); e != nil {
			t.Fatal(e)
		}
		if e = writer.Close(); e != nil {
			t.Fatal(e)
		}
		return b.request("POST", "/admin/themes/upload", body.String(), writer.FormDataContentType(), false).Code
	}
	data := themeZip(t, themeFiles())
	if got := upload(data, false); got != 403 {
		t.Fatalf("upload without CSRF: %d", got)
	}
	if got := upload(data, true); got != 303 {
		t.Fatalf("upload: %d", got)
	}
	if got := upload(data, true); got != 400 {
		t.Fatalf("duplicate upload: %d", got)
	}
	current, err := s.Appearance()
	if err != nil || current.Theme != "journal" {
		t.Fatal("installation activated theme", current, err)
	}
	page := b.request("GET", "/admin/themes", "", "", false)
	status(t, page, 200)
	for _, marker := range []string{"Моя тема", "theme-catalog", "<img", "/admin/themes/screenshot?theme=", "Активировать", "/admin/themes/upload"} {
		if !strings.Contains(page.Body.String(), marker) {
			t.Errorf("catalog missing %s", marker)
		}
	}
	if strings.Contains(page.Body.String(), "<iframe") || strings.Contains(page.Body.String(), "Настройки главной") || strings.Contains(page.Body.String(), "home_title") || !strings.Contains(page.Body.String(), "Шаблоны") {
		t.Fatal("catalog still contains obsolete settings or nested pages")
	}
	for _, id := range []string{"journal", "minimal", "theme:test-theme"} {
		image := b.request("GET", "/admin/themes/screenshot?theme="+id, "", "", false)
		status(t, image, 200)
		if !strings.Contains(image.Header().Get("Content-Type"), "image/svg+xml") || !strings.Contains(image.Body.String(), "<svg") || !strings.Contains(image.Header().Get("Content-Security-Policy"), "script-src 'none'") {
			t.Fatal("invalid screenshot response")
		}
	}

	if strings.Contains(page.Body.String(), "theme-swatch") {
		t.Fatal("wireframe remains")
	}
	preview := b.request("GET", "/admin/themes/view?theme=theme:test-theme&thumbnail=1", "", "", false)
	status(t, preview, 200)
	if !strings.Contains(preview.Body.String(), "MY CUSTOM HOME") || !strings.Contains(preview.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("unsafe or empty thumbnail")
	}
	status(t, b.request("GET", "/theme-assets/test-theme/style.css", "", "", false), 200)
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("GET", "/admin/themes/view?theme=journal", "", "", false), 401)
	if !strings.Contains(preview.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatal("thumbnail indexed")
	}
	status(t, b.request("GET", "/theme-assets/test-theme/", "", "", false), 404)
	status(t, b.request("GET", "/theme-assets/test-theme/../theme.json", "", "", false), 404)
	fields := url.Values{"csrf": {b.cookies["csrf"].Value}, "scope": {"theme"}, "intent": {"activate"}, "version": {"1"}, "theme": {"theme:test-theme"}}
	status(t, b.request("POST", "/admin/themes", fields.Encode(), "application/x-www-form-urlencoded", false), 303)
	current, err = s.Appearance()
	if err != nil || current.Theme != "theme:test-theme" || current.HomeTitle != "Keep home" || current.SEO.Title != "Keep SEO" {
		t.Fatal("activation lost settings", current, err)
	}
	status(t, b.request("POST", "/admin/themes", fields.Encode(), "application/x-www-form-urlencoded", false), 409)
	public := b.request("GET", "/", "", "", false)
	status(t, public, 200)
	if !strings.Contains(public.Body.String(), `href="/theme-assets/test-theme/style.css"`) {
		t.Fatal("theme assets URL missing")
	}
	removal := url.Values{"csrf": {b.cookies["csrf"].Value}, "theme": {"theme:test-theme"}}
	status(t, b.request("POST", "/admin/themes/delete", removal.Encode(), "application/x-www-form-urlencoded", false), 400)
	fields.Set("theme", "minimal")
	fields.Set("version", "2")
	status(t, b.request("POST", "/admin/themes", fields.Encode(), "application/x-www-form-urlencoded", false), 303)
	status(t, b.request("POST", "/admin/themes/delete", removal.Encode(), "application/x-www-form-urlencoded", false), 303)
	status(t, b.request("GET", "/theme-assets/test-theme/style.css", "", "", false), 404)
	if _, err = s.DB.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	// Themes are inside the existing backup tree; no separate backup format is needed.
	dir := s.Dir
	s.Close()
	dest := filepath.Join(t.TempDir(), "backup")
	if err = backup.Create(dir, dest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, "theme", "packages", "my-journal", "theme.json")); err != nil {
		t.Fatal("installed theme absent from backup", err)
	}
	if err = backup.Verify(dest); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err = backup.Restore(dest, restored); err != nil {
		t.Fatal(err)
	}
	restoredStore, restoredApp := openApp(t, restored)
	defer restoredStore.Close()
	if _, _, mode, e := restoredApp.themeSource("theme:my-journal"); e != nil || mode != "theme:my-journal" {
		t.Fatal("restored theme unavailable", mode, e)
	}
}

func TestThemeArchivesRejectUnsafeOrBrokenPackages(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	for name, edit := range map[string]func(map[string]string){
		"traversal":             func(f map[string]string) { f["../outside.css"] = "bad" },
		"absolute":              func(f map[string]string) { f["/outside.css"] = "bad" },
		"windows":               func(f map[string]string) { f[`assets\evil.css`] = "bad" },
		"php":                   func(f map[string]string) { f["index.php"] = "bad" },
		"bad-id":                func(f map[string]string) { f["theme.json"] = `{"id":"../bad","name":"Bad"}` },
		"missing-screenshot":    func(f map[string]string) { delete(f, "screenshot.svg") },
		"invalid-screenshot":    func(f map[string]string) { f["screenshot.svg"] = "not svg" },
		"mismatched-screenshot": func(f map[string]string) { f["screenshot.png"] = f["screenshot.svg"]; delete(f, "screenshot.svg") },
		"missing-layout":        func(f map[string]string) { delete(f, "layout.html") },
		"broken-template":       func(f map[string]string) { f["page.html"] = "{{define" },
		"execution-error":       func(f map[string]string) { f["page.html"] = `{{define "body"}}{{.NotAField}}{{end}}` },
		"oversize":              func(f map[string]string) { f["assets/style.css"] = strings.Repeat("x", 8<<20+1) },
	} {
		t.Run(name, func(t *testing.T) {
			files := themeFiles()
			edit(files)
			if _, err := app.installTheme(bytes.NewReader(themeZip(t, files))); err == nil {
				t.Fatal("unsafe theme accepted")
			}
			entries, err := os.ReadDir(filepath.Join(s.Dir, "theme", "packages"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("failed installation left files")
			}
		})
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	header := &zip.FileHeader{Name: "assets/link.css"}
	header.SetMode(os.ModeSymlink | 0777)
	file, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(file, "../../outside"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = app.installTheme(bytes.NewReader(out.Bytes())); err == nil {
		t.Fatal("symlink accepted")
	}
	wrapped := map[string]string{}
	for name, content := range themeFiles() {
		wrapped["my-theme/"+name] = content
	}
	if _, err = app.installTheme(bytes.NewReader(themeZip(t, wrapped))); err != nil {
		t.Fatal("single folder package rejected", err)
	}
}

func TestScreenshotValidation(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	if err := validateScreenshot(pngData.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := validateScreenshot(pngData.Bytes(), "image/jpeg"); err == nil {
		t.Fatal("mismatched image accepted")
	}
	if err := validateScreenshot([]byte(`<html/>`), "image/svg+xml"); err == nil {
		t.Fatal("HTML preview accepted")
	}
}
