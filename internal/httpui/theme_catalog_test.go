package httpui

import (
	"context"
	"encoding/json"
	"gopress/internal/i18n"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestBuiltinThemeSwitchRefreshesActiveCardAndPublicDesign(t *testing.T) {
	s, app, b := seoApp(t)
	for _, id := range []string{"minimal", "studio", "journal"} {
		current, err := s.Appearance()
		if err != nil {
			t.Fatal(err)
		}
		fields := url.Values{"scope": {"theme"}, "intent": {"activate"}, "theme": {id}, "version": {strconv.FormatInt(current.Version, 10)}}
		response := adminHTML(b, "POST", "/admin/themes", fields, true, false, true)
		status(t, response, 204)
		var navigation struct{ Path, Target, Select, Swap string }
		if err = json.Unmarshal([]byte(response.Header().Get("HX-Location")), &navigation); err != nil {
			t.Fatal(err)
		}
		if navigation.Path != "/admin/themes?saved=1" || navigation.Target != "#main" || navigation.Select != "#main" || navigation.Swap != "outerHTML show:top" {
			t.Fatal("activation must replace main, never the admin body", navigation)
		}
		response = adminHTML(b, "GET", navigation.Path, nil, true, false, false)
		if !strings.Contains(response.Body.String(), `id="main"`) || strings.Contains(response.Body.String(), `data-slot="sidebar-wrapper"`) {
			t.Fatal("expected main-only activation fragment")
		}
		full := adminHTML(b, "GET", navigation.Path, nil, false, false, false)
		if !strings.Contains(full.Body.String(), `data-slot="sidebar-wrapper"`) {
			t.Fatal("full admin shell lost its sidebar")
		}

		status(t, response, 200)
		html := response.Body.String()
		matches := regexp.MustCompile(`data-theme-id="([^"]+)" data-active="([^"]+)"`).FindAllStringSubmatch(html, -1)
		if len(matches) != 3 {
			t.Fatal("expected three builtin cards", html)
		}
		active := 0
		for i, want := range []string{"journal", "minimal", "studio"} {
			if matches[i][1] != want {
				t.Fatal("card order changed", matches)
			}
			if matches[i][2] == "true" {
				active++
				if matches[i][1] != id {
					t.Fatal("wrong active theme", matches)
				}
			}
		}
		if active != 1 || !strings.Contains(html, "theme-current-button") {
			t.Fatal("unclear active state", html)
		}
		public := b.request("GET", "/", "", "", false)
		status(t, public, 200)
		if !strings.Contains(public.Body.String(), `data-theme="`+id+`"`) {
			t.Fatal("public theme did not change", id)
		}
		css := "public.css"
		if id != "journal" {
			css = "public-" + id + ".css"
		}
		if !strings.Contains(public.Body.String(), "/assets/"+css) {
			t.Fatal("same stylesheet", id)
		}
		status(t, b.request("GET", "/assets/"+css, "", "", false), 200)
		status(t, b.request("GET", "/admin/themes/view?theme="+id, "", "", false), 200)
		preview := b.request("GET", "/admin/themes/screenshot?theme="+id, "", "", false)
		status(t, preview, 200)
		if err = validateScreenshot(preview.Body.Bytes(), "image/svg+xml"); err != nil {
			t.Fatal(id, err)
		}
	}
	// A stale request must keep showing the saved theme, not the requested one.
	response := adminHTML(b, "POST", "/admin/themes", url.Values{"scope": {"theme"}, "intent": {"activate"}, "theme": {"studio"}, "version": {"0"}}, true, false, true)
	status(t, response, 409)
	if !strings.Contains(response.Body.String(), `data-theme-id="journal" data-active="true"`) {
		t.Fatal("failed switch changed visible active state")
	}
	if err := app.validateTheme(store.Appearance{Theme: "studio"}); err != nil {
		t.Fatal(err)
	}
}

func TestThemeUploadHasDedicatedPage(t *testing.T) {
	_, app, b := seoApp(t)
	catalog := b.request("GET", "/admin/themes", "", "", false)
	status(t, catalog, 200)
	if !strings.Contains(catalog.Body.String(), "Загрузить свой шаблон") || strings.Contains(catalog.Body.String(), `name="theme_zip"`) {
		t.Fatal("upload form still occupies catalog")
	}
	upload := b.request("GET", "/admin/themes/upload", "", "", false)
	status(t, upload, 200)
	for _, part := range []string{`name="theme_zip"`, `enctype="multipart/form-data"`, `name="csrf"`, "Установить"} {
		if !strings.Contains(upload.Body.String(), part) {
			t.Fatal(part)
		}
	}
	anon := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, anon.request("GET", "/admin/themes/upload", "", "", false), 401)
}

func TestTemplateTerminologyInEnglish(t *testing.T) {
	ctx := i18n.WithLanguage(context.Background(), "en")
	for source, want := range map[string]string{"Шаблоны": "Templates", "Шаблон": "Template", "Активный шаблон:": "Active template:", "Загрузить свой шаблон": "Upload your template"} {
		if got := i18n.T(ctx, source); got != want {
			t.Fatalf("%s: %s", source, got)
		}
	}
}
