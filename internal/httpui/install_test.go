package httpui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/service"
)

func TestInstallationLinkUnlocksSetupAndIsDisabledAfterInstall(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	token := app.Service.InstallToken
	page := b.request("GET", "/install", "", "", false)
	status(t, page, 200)
	if strings.Contains(page.Body.String(), token) || strings.Contains(page.Body.String(), `name="token"`) || strings.Contains(page.Body.String(), `name="password"`) {
		t.Fatal("locked installer leaked credentials or exposed setup")
	}
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("installer can be cached")
	}
	unlock := url.Values{"token": {token}}
	status(t, b.request("POST", "/install/unlock", unlock.Encode(), "application/x-www-form-urlencoded", false), 403)
	status(t, b.request("POST", "/install/unlock", "token=invalid", "application/x-www-form-urlencoded", true), 401)
	fields := url.Values{"title": {"My new site"}, "base_url": {"https://example.com"}, "timezone": {"UTC"}, "email": {"owner@example.com"}, "password": {"correct horse battery staple"}}
	status(t, b.request("POST", "/install", fields.Encode(), "application/x-www-form-urlencoded", true), 401)
	status(t, b.request("POST", "/install/unlock", unlock.Encode(), "application/x-www-form-urlencoded", true), 204)
	grant := b.cookies["installation"]
	if grant == nil || !grant.HttpOnly || grant.SameSite != http.SameSiteStrictMode || grant.MaxAge != 1800 {
		t.Fatal("unsafe installation grant")
	}
	page = b.request("GET", "/install", "", "", false)
	status(t, page, 200)
	if !strings.Contains(page.Body.String(), `name="title"`) || strings.Contains(page.Body.String(), token) || strings.Contains(page.Body.String(), `name="token"`) {
		t.Fatal("setup was not unlocked privately")
	}
	fields.Set("timezone", "invalid")
	failed := b.request("POST", "/install", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, failed, 400)
	for _, value := range []string{"My new site", "https://example.com", "owner@example.com", `value="invalid"`} {
		if !strings.Contains(failed.Body.String(), value) {
			t.Fatal("validation discarded input", value)
		}
	}
	if strings.Contains(failed.Body.String(), fields.Get("password")) {
		t.Fatal("password echoed into HTML")
	}
	fields.Set("timezone", "UTC")
	done := b.request("POST", "/install", fields.Encode(), "application/x-www-form-urlencoded", true)
	status(t, done, 303)
	if done.Header().Get("Location") != "/login" || b.cookies["installation"].MaxAge != -1 {
		t.Fatal("temporary installation access was not cleared")
	}
	b.cookies["installation"] = grant
	status(t, b.request("GET", "/install", "", "", false), 409)
	status(t, b.request("POST", "/install/unlock", unlock.Encode(), "application/x-www-form-urlencoded", true), 409)
	status(t, b.request("POST", "/install", fields.Encode(), "application/x-www-form-urlencoded", true), 409)
	restarted, err := service.New(s)
	if err != nil || restarted.InstallToken != "" || !restarted.Installed() {
		t.Fatal("restart reopened installation", err)
	}
}

func TestInstallationGrantIsSecureBehindHTTPSProxy(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/install", "", "", false), 200)
	for _, origin := range []string{"https://example.com", "https://attacker.test"} {
		r := httptest.NewRequest(http.MethodPost, "http://example.com/install/unlock", strings.NewReader(url.Values{"token": {app.Service.InstallToken}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", b.csrf)
		r.AddCookie(b.cookies["csrf"])
		w := httptest.NewRecorder()
		app.Echo.ServeHTTP(w, r)
		if origin != "https://example.com" {
			status(t, w, 403)
			continue
		}
		status(t, w, 204)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
			t.Fatal("setup grant was not protected on HTTPS")
		}
	}
}
