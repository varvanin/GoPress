package httpui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

func TestSettingsHomepageAndLanguage(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	for _, tab := range []string{"general", "home", "mail", "language"} {
		status(t, adminHTML(b, "GET", "/admin/settings?tab="+tab, nil, false, false, false), 200)
	}
	settings, _ := s.Settings()
	fields := url.Values{"version": {fmt.Sprint(settings.Version)}, "title": {"Changed"}, "base_url": {"https://example.com"}, "timezone": {"Europe/Warsaw"}, "description": {"Site description"}}
	status(t, adminHTML(b, "POST", "/admin/settings/general", fields, true, false, false), 403)
	savedResponse := adminHTML(b, "POST", "/admin/settings/general", fields, true, false, true)
	status(t, savedResponse, 204)
	if savedResponse.Header().Get("HX-Trigger") != "settingsSaved" {
		t.Fatal("missing saved event before full reload")
	}
	status(t, adminHTML(b, "POST", "/admin/settings/general", fields, true, false, true), 409)
	home, err := app.Service.Save(ctx, store.Page{Title: "Home snapshot", Slug: "landing", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	home.Title = "Unpublished changes"
	if _, err = app.Service.Save(ctx, home, false); err != nil {
		t.Fatal(err)
	}
	settings, _ = s.Settings()
	fields = url.Values{"version": {fmt.Sprint(settings.Version)}, "home_mode": {"page"}, "home_page_id": {fmt.Sprint(home.ID)}, "posts_per_page": {"2"}}
	status(t, adminHTML(b, "POST", "/admin/settings/home", fields, true, false, true), 204)
	response := b.request("GET", "/", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Home snapshot") || strings.Contains(response.Body.String(), "Unpublished changes") || !strings.Contains(response.Body.String(), `rel="canonical" href="https://example.com/"`) {
		t.Fatal("static homepage did not use published snapshot/canonical")
	}
	currentHome, _ := s.Page(home.ID)
	if err = s.ChangeContent(ctx, currentHome.ID, currentHome.Version, "unpublish"); err != nil {
		t.Fatal(err)
	}
	fallback := b.request("GET", "/", "", "", false)
	status(t, fallback, 200)
	if !strings.Contains(fallback.Body.String(), `id="journal"`) || strings.Contains(fallback.Body.String(), "Unpublished changes") {
		t.Fatal("missing homepage fallback")
	}
	settings, _ = s.Settings()
	settings.Description = "Description after unpublishing home"
	if err = app.Service.SaveSettings(ctx, settings); err != nil {
		t.Fatal("unpublished homepage blocked unrelated settings", err)
	}
	settings, _ = s.Settings()
	fields = url.Values{"version": {fmt.Sprint(settings.Version)}, "home_mode": {"posts"}, "home_page_id": {"0"}, "posts_per_page": {"2"}}
	status(t, adminHTML(b, "POST", "/admin/settings/home", fields, true, false, true), 204)
	for i := 0; i < 3; i++ {
		if _, err = app.Service.Save(ctx, store.Page{Kind: "post", Title: fmt.Sprintf("Post %d", i), Slug: fmt.Sprintf("settings-post-%d", i), Document: document}, true); err != nil {
			t.Fatal(err)
		}
	}
	posts, total, err := s.PublicPosts(0, 1, 2)
	if err != nil || len(posts) != 2 || total != 3 {
		t.Fatal("page size ignored", len(posts), total, err)
	}
	response = b.request("GET", "/", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), `/?page=2`) {
		t.Fatal("homepage pagination ignores size")
	}
	if _, err = app.Service.Save(ctx, store.Page{Title: "Настройки", Slug: "user-owned-title", Document: document}, true); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "pl", "ru", "de", "uk", "it", "es", "cs"} {
		settings, _ = s.Settings()
		fields = url.Values{"version": {fmt.Sprint(settings.Version)}, "language": {language}}
		status(t, adminHTML(b, "POST", "/admin/settings/language", fields, true, false, true), 204)
		response = adminHTML(b, "GET", "/admin/settings?tab=general", nil, false, false, false)
		status(t, response, 200)
		expected := map[string]string{"en": "General settings", "pl": "Ustawienia ogólne", "ru": "Общие настройки", "de": "Allgemeine Einstellungen", "uk": "Загальні налаштування", "it": "Impostazioni generali", "es": "Configuración general", "cs": "Obecná nastavení"}[language]
		if !strings.Contains(response.Body.String(), expected) || !strings.Contains(response.Body.String(), `lang="`+language+`"`) {
			t.Fatal("language not applied", language)
		}
		listing := adminHTML(b, "GET", "/admin/pages", nil, false, false, false)
		if !strings.Contains(listing.Body.String(), ">Настройки</a>") {
			t.Fatal("translated user content", language)
		}
		login := b.request("GET", "/login", "", "", false)
		status(t, login, 200)
		if !strings.Contains(login.Body.String(), `lang="`+language+`"`) {
			t.Fatal("login language")
		}
		scripts := b.request("GET", "/admin/translations.js", "", "", false)
		status(t, scripts, 200)
		if !strings.Contains(scripts.Body.String(), "GoPressTranslations=") || language != "ru" && !strings.Contains(scripts.Body.String(), i18n.T(i18n.WithLanguage(context.Background(), language), "Настройки")) {
			t.Fatal("missing JS catalog")
		}
	}
}
func TestSettingsMailNeverEchoesSecrets(t *testing.T) {
	t.Setenv("GOPRESS_SMTP_HOST", "")
	t.Setenv("GOPRESS_SMTP_FROM", "")
	_, app, b := seoApp(t)
	fields := url.Values{"enabled": {"true"}, "host": {"smtp.example.com"}, "port": {"587"}, "from": {"site@example.com"}, "username": {"user"}, "password": {"SECRET-MUST-NOT-APPEAR"}}
	status(t, adminHTML(b, "POST", "/admin/settings/mail", fields, true, false, false), 403)
	status(t, adminHTML(b, "POST", "/admin/settings/mail", fields, true, false, true), 204)
	response := adminHTML(b, "GET", "/admin/settings?tab=mail", nil, false, false, false)
	status(t, response, 200)
	if strings.Contains(response.Body.String(), "SECRET-MUST-NOT-APPEAR") || !strings.Contains(response.Body.String(), "Пароль сохранён") {
		t.Fatal("password exposed or missing status")
	}
	currentMail, err := app.Service.MailSettings()
	if err != nil {
		t.Fatal(err)
	}
	fields.Set("mail_version", currentMail.Version)
	fields.Set("host", "draft.example.com")
	fields.Set("port", "0")
	fields.Set("password", "UNSAVED-SECRET")
	invalid := adminHTML(b, "POST", "/admin/settings/mail", fields, true, false, true)
	status(t, invalid, 400)
	if !strings.Contains(invalid.Body.String(), `value="draft.example.com"`) || strings.Contains(invalid.Body.String(), "UNSAVED-SECRET") || strings.Contains(invalid.Body.String(), "SECRET-MUST-NOT-APPEAR") || !strings.Contains(invalid.Body.String(), `data-form-dirty="true"`) {
		t.Fatal("invalid mail settings lost values or exposed a secret")
	}
	status(t, adminHTML(b, "POST", "/admin/settings/mail-test", url.Values{"test_recipient": {"invalid"}}, true, false, true), 400)
}

func TestTranslatedButtonLabelsRemainVisible(t *testing.T) {
	for _, language := range []string{"ru", "en", "pl", "de", "uk", "it", "es", "cs"} {
		var out strings.Builder
		ctx := i18n.WithLanguage(context.Background(), language)
		if err := Pages("Site", "token", []store.Page{{ID: 1, Title: "User content", Slug: "user", Kind: "page"}}, ContentList{Kind: "page", Total: 1}).Render(ctx, &out); err != nil {
			t.Fatal(err)
		}
		newPage := i18n.T(ctx, "Новая страница")
		if !strings.Contains(out.String(), "<span>"+newPage+"</span>") {
			t.Fatal("new page label swallowed by icon", language)
		}
		out.Reset()
		if err := Forms("Site", "token", nil, false, store.Settings{FormRetentionDays: 90}).Render(ctx, &out); err != nil {
			t.Fatal(err)
		}
		create := i18n.T(ctx, "Создать форму")
		if !strings.Contains(out.String(), "<span>"+create+"</span>") {
			t.Fatal("create form label swallowed by icon", language)
		}
	}
}

func TestAdminSectionForEveryLanguage(t *testing.T) {
	for _, language := range []string{"ru", "en", "pl", "de", "uk", "it", "es", "cs"} {
		ctx := i18n.WithLanguage(context.Background(), language)
		for title, want := range map[string]string{"Пользователи": "/admin/users", "Мой профиль": "/admin/account", "Шаблоны": "/admin/themes", "Модули": "/admin/modules", "Инструменты": "/admin/tools", "Настройки": "/admin/settings", "Формы": "/admin/forms", "Медиатека": "/admin/media", "Меню": "/admin/menus", "Категории и теги": "/admin/posts/terms", "Записи": "/admin/posts", "Страницы": "/admin/pages"} {
			translated := i18n.T(ctx, title)
			if got := adminSection(translated); got != want {
				t.Fatalf("%s: %s (%s): %s != %s", language, title, translated, got, want)
			}
		}
	}
}
