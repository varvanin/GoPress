package httpui

import (
	"strings"
	"testing"
)

func TestAdminAppearanceLoadsBeforeStylesAndStaysOutOfPublicTemplates(t *testing.T) {
	_, _, b := seoApp(t)
	full := b.request("GET", "/admin", "", "", false)
	status(t, full, 200)
	body := full.Body.String()
	script := strings.Index(body, `src="/assets/admin-theme.js"`)
	css := strings.Index(body, `href="/assets/admin.css"`)
	if script < 0 || css < script || !strings.Contains(body, "data-admin-theme-toggle") {
		t.Fatal("theme can flash or toggle missing")
	}
	partial := adminHTML(b, "GET", "/admin/seo", nil, true, false, false)
	status(t, partial, 200)
	if strings.Contains(partial.Body.String(), `src="/assets/admin-theme.js"`) {
		t.Fatal("partial navigation reinitializes appearance")
	}
	login := b.request("GET", "/login", "", "", false)
	status(t, login, 200)
	if !strings.Contains(login.Body.String(), "gp-theme-toggle-compact") {
		t.Fatal("login lacks appearance toggle")
	}
	asset := b.request("GET", "/assets/admin-theme.js", "", "", false)
	status(t, asset, 200)
	public := b.request("GET", "/", "", "", false)
	status(t, public, 200)
	if strings.Contains(public.Body.String(), "admin-theme.js") || strings.Contains(public.Body.String(), "data-admin-theme-toggle") {
		t.Fatal("admin appearance leaks into public templates")
	}
}
