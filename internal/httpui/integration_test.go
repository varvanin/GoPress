package httpui

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/service"
	"gopress/internal/store"
)

const document = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"First public revision <script>alert(1)</script>"}]}]}`

type browser struct {
	app     *App
	cookies map[string]*http.Cookie
	csrf    string
}

func (b *browser) request(method, path, body, content string, withCSRF bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	if content != "" {
		r.Header.Set("Content-Type", content)
	}
	for _, cookie := range b.cookies {
		r.AddCookie(cookie)
	}
	if withCSRF {
		r.Header.Set("X-CSRF-Token", b.csrf)
	}
	w := httptest.NewRecorder()
	b.app.Echo.ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		b.cookies[cookie.Name] = cookie
		if cookie.Name == "csrf" {
			b.csrf = cookie.Value
		}
	}
	return w
}
func status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
	}
}
func openApp(t *testing.T, dir string) (*store.Store, *App) {
	t.Helper()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := service.New(s)
	if err != nil {
		t.Fatal(err)
	}
	return s, New(v)
}
func TestFullBackendScenario(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	dir := filepath.Join(t.TempDir(), "site")
	s, app := openApp(t, dir)
	defer func() { s.Close() }()
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("POST", "/admin/pages", "{}", "application/json", false), 401)
	status(t, b.request("GET", "/install", "", "", false), 200)
	form := url.Values{"token": {app.Service.InstallToken}, "title": {"Test Site"}, "base_url": {"http://example.com"}, "timezone": {"Europe/Warsaw"}, "email": {"owner@example.com"}, "password": {"correct horse battery staple"}}
	status(t, b.request("POST", "/install", form.Encode(), "application/x-www-form-urlencoded", false), 403)
	wrong := form.Get("token")
	form.Set("token", "wrong")
	status(t, b.request("POST", "/install", form.Encode(), "application/x-www-form-urlencoded", true), 401)
	form.Set("token", wrong)
	// Actual concurrent HTTP requests, each with the same valid installation credential.
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clone := &browser{app: app, cookies: map[string]*http.Cookie{"csrf": b.cookies["csrf"]}, csrf: b.csrf}
			codes <- clone.request("POST", "/install", form.Encode(), "application/x-www-form-urlencoded", true).Code
		}()
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		switch code {
		case 303:
			success++
		case 409:
			conflict++
		default:
			t.Fatalf("installation: %d", code)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("installation was not atomic")
	}
	var users int
	if err := s.DB.QueryRow("SELECT count(*) FROM users").Scan(&users); err != nil || users != 1 {
		t.Fatalf("users=%d err=%v", users, err)
	}
	status(t, b.request("POST", "/install", form.Encode(), "application/x-www-form-urlencoded", true), 409)
	status(t, b.request("GET", "/install", "", "", false), 409)
	login := url.Values{"email": {"owner@example.com"}, "password": {"correct horse battery staple"}}
	status(t, b.request("POST", "/login", login.Encode(), "application/x-www-form-urlencoded", true), 303)
	if !b.cookies["session"].HttpOnly || b.cookies["session"].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	var rawSessions int
	if err := s.DB.QueryRow("SELECT count(*) FROM sessions WHERE hash=?", b.cookies["session"].Value).Scan(&rawSessions); err != nil || rawSessions != 0 {
		t.Fatal("raw session in DB")
	}
	p := store.Page{Title: "A real page", Slug: "hello", Document: document}
	raw, _ := json.Marshal(p)
	status(t, b.request("POST", "/admin/pages", string(raw), "application/json", false), 403)
	injected := &browser{app: app, cookies: map[string]*http.Cookie{"session": b.cookies["session"], "csrf": {Name: "csrf", Value: strings.Repeat("a", 64)}}, csrf: strings.Repeat("a", 64)}
	status(t, injected.request("POST", "/admin/pages", string(raw), "application/json", true), 403)
	w := b.request("POST", "/admin/pages", string(raw), "application/json", true)
	status(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	status(t, anon.request("GET", "/hello", "", "", false), 404)
	status(t, anon.request("GET", fmt.Sprintf("/admin/pages/%d/preview", p.ID), "", "", false), 401)
	w = b.request("GET", fmt.Sprintf("/admin/pages/%d/preview", p.ID), "", "", false)
	status(t, w, 200)
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("X-Robots-Tag"), "noindex") || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("unsafe preview")
	}
	imageBytes, err := os.ReadFile("../../scripts/fixtures/smoke.png")
	if err != nil {
		t.Fatal(err)
	}
	w = b.request("POST", "/admin/media", string(imageBytes), "image/png", true)
	status(t, w, 201)
	var media struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &media); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<svg onload='alert(1)'/>", "<html>bad</html>", "not an image", strings.Repeat("x", service.MaxUpload+1)} {
		status(t, b.request("POST", "/admin/media", bad, "image/png", true), 400)
	}
	p.Cover = media.Name
	raw, _ = json.Marshal(p)
	w = b.request("POST", fmt.Sprintf("/admin/pages/%d", p.ID), string(raw), "application/json", true)
	status(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("POST", fmt.Sprintf("/admin/pages/%d", p.ID), string(raw), "application/json", true), 409)
	publish := fmt.Sprintf("/admin/pages/%d/publish", p.ID)
	status(t, b.request("POST", publish, fmt.Sprintf(`{"version":%d}`, p.Version), "application/json", true), 204)
	p.Version++
	w = anon.request("GET", "/hello", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "First public revision") || !strings.Contains(w.Body.String(), media.Name) || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("public render")
	}
	p.Title = "PRIVATE DRAFT"
	p.Document = strings.ReplaceAll(document, "First public revision", "SECRET DRAFT")
	p.Slug = "new-address"
	raw, _ = json.Marshal(p)
	w = b.request("POST", fmt.Sprintf("/admin/pages/%d", p.ID), string(raw), "application/json", true)
	status(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	w = anon.request("GET", "/hello", "", "", false)
	status(t, w, 200)
	if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal("draft leaked")
	}
	status(t, anon.request("GET", "/new-address", "", "", false), 404)
	for _, slug := range []string{"hello", "new-address", "admin", "api", "theme-assets", "module-assets", "uploads", "install", "Bad Slug", "../x"} {
		q := store.Page{Title: "Collision", Slug: slug, Document: document}
		raw, _ = json.Marshal(q)
		w = b.request("POST", "/admin/pages", string(raw), "application/json", true)
		if w.Code != 400 && w.Code != 409 {
			t.Fatalf("slug %s accepted: %d", slug, w.Code)
		}
	}
	p.Document = `{"type":"doc","content":[{"type":"script"}]}`
	raw, _ = json.Marshal(p)
	status(t, b.request("POST", fmt.Sprintf("/admin/pages/%d", p.ID), string(raw), "application/json", true), 400)
	status(t, anon.request("GET", "/missing", "", "", false), 404)
	publicBeforeBackup := anon.request("GET", "/hello", "", "", false).Body.String()
	uploaded := anon.request("GET", "/uploads/"+media.Name, "", "", false)
	status(t, uploaded, 200)
	if uploaded.Header().Get("Content-Type") != "image/png" || len(uploaded.Body.Bytes()) == 0 {
		t.Fatal("uploaded image was not served as PNG")
	}
	draftBeforeBackup, err := s.Page(p.ID)
	if err != nil || draftBeforeBackup.Title != "PRIVATE DRAFT" || !strings.Contains(draftBeforeBackup.Document, "SECRET DRAFT") {
		t.Fatal("working draft was lost", err)
	}
	snapshot := filepath.Join(t.TempDir(), "backup")
	if err := backup.Create(dir, snapshot); err == nil {
		t.Fatal("backup while server owns lock")
	}
	if _, err := store.Open(dir); err == nil {
		t.Fatal("second instance acquired data lock")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, app = openApp(t, dir)
	b.app = app
	anon.app = app
	w = anon.request("GET", "/hello", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "First public revision") {
		t.Fatal("restart lost published revision")
	}
	status(t, b.request("GET", "/admin/pages", "", "", false), 200)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := backup.Create(dir, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := backup.Verify(snapshot); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err := backup.Restore(snapshot, restored); err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(snapshot, restored); err == nil {
		t.Fatal("restore overwrote destination")
	}
	s, app = openApp(t, restored)
	b.app = app
	anon.app = app
	w = anon.request("GET", "/hello", "", "", false)
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), "First public revision") {
		t.Fatal("restore lost content")
	}
	restoredImage := anon.request("GET", "/uploads/"+media.Name, "", "", false)
	status(t, restoredImage, 200)
	if !bytes.Equal(restoredImage.Body.Bytes(), uploaded.Body.Bytes()) || restoredImage.Header().Get("Content-Type") != "image/png" {
		t.Fatal("restore changed uploaded image")
	}
	if w.Body.String() != publicBeforeBackup {
		t.Fatal("restore changed public page")
	}
	restoredDraft, err := s.Page(p.ID)
	if err != nil || restoredDraft.Title != draftBeforeBackup.Title || restoredDraft.Document != draftBeforeBackup.Document || restoredDraft.Slug != draftBeforeBackup.Slug || restoredDraft.Cover != draftBeforeBackup.Cover || restoredDraft.Version != draftBeforeBackup.Version {
		t.Fatal("restore changed working revision", err)
	}
	status(t, b.request("GET", "/admin/pages", "", "", false), 401)
	if _, err := s.Public("new-address"); err != sql.ErrNoRows {
		t.Fatal("draft published by restore")
	}
	if _, err := s.Page(p.ID); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", login.Encode(), "application/x-www-form-urlencoded", true), 303)
	status(t, b.request("POST", "/logout", "", "", true), 303)
	status(t, b.request("GET", "/admin/pages", "", "", false), 401)
	if err := os.WriteFile(filepath.Join(snapshot, "uploads", media.Name), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := backup.Verify(snapshot); err == nil {
		t.Fatal("tampered backup verified")
	}
}
func TestCorruptDatabaseNeverInstalls(t *testing.T) {
	for _, data := range [][]byte{[]byte("corrupt database"), {}} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "site.db"), data, 0600); err != nil {
			t.Fatal(err)
		}
		s, err := store.Open(dir)
		if err == nil {
			s.Close()
			t.Fatal("existing corrupt or unknown DB accepted")
		}
	}
}
func TestPragmasEveryConnection(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.DB.SetMaxIdleConns(0)
	for range 2 {
		for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000, "synchronous": 2} {
			var n int
			if err = s.DB.QueryRow("PRAGMA " + pragma).Scan(&n); err != nil || n != want {
				t.Fatalf("%s=%d err=%v", pragma, n, err)
			}
		}
	}
}
