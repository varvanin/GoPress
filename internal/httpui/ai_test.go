package httpui

import (
	"context"
	"gopress/internal/service"
	"gopress/internal/store"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAISettingsStayPrivateAndEditorRequiresCSRF(t *testing.T) {
	_, app, b := seoApp(t)
	status(t, b.request("GET", "/admin/ai", "", "", false), 200)
	value, _ := app.Service.AISettings()
	form := url.Values{"version": {value.Version}, "model": {"openrouter/auto"}, "key": {"private-ai-key"}}.Encode()
	status(t, b.request("POST", "/admin/ai/settings", form, "application/x-www-form-urlencoded", false), 403)
	status(t, b.request("POST", "/admin/ai/settings", form, "application/x-www-form-urlencoded", true), 303)
	response := b.request("GET", "/admin/ai", "", "", false)
	if strings.Contains(response.Body.String(), "private-ai-key") || !strings.Contains(response.Body.String(), "clear_key") {
		t.Fatal("key exposed or saved state missing")
	}
	form = url.Values{"version": {value.Version}, "model": {"bad model"}, "key": {"another-private-key"}}.Encode()
	response = b.request("POST", "/admin/ai/settings", form, "application/x-www-form-urlencoded", true)
	status(t, response, 409)
	if strings.Contains(response.Body.String(), "private-key") {
		t.Fatal("error form exposed key")
	}
	status(t, b.request("POST", "/admin/pages/ai", `{"prompt":"Write","language":"ru"}`, "application/json", false), 403)
	status(t, b.request("POST", "/admin/pages/ai", `{"prompt":"Write","language":"ru","text":"`+strings.Repeat("x", 100000)+`"}`, "application/json", true), 400)
	page := b.request("GET", "/admin/pages/new", "", "", false).Body.String()
	if !strings.Contains(page, "data-editor-ai") || strings.Contains(page, "private-ai-key") {
		t.Fatal("editor AI panel missing or exposes key")
	}
	id, err := app.Service.Store.SaveUser(context.Background(), store.User{Email: "author@example.com", Role: "author"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	token := service.Token()
	if err = app.Service.Store.LoginSession(service.Hash(token), id, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	b.cookies["session"] = &http.Cookie{Name: "session", Value: token}
	status(t, b.request("GET", "/admin/ai", "", "", false), 403)
	status(t, b.request("POST", "/admin/ai/settings", form, "application/x-www-form-urlencoded", true), 403)
	status(t, b.request("GET", "/admin/posts/new", "", "", false), 200)
	// An author can use the helper, but cannot configure its shared credentials.
	actor := store.WithActor(context.Background(), store.User{ID: id, Role: "author"})
	if err = app.Service.SaveAISettings(actor, service.AISettings{}, false); err != service.ErrForbidden {
		t.Fatal(err)
	}
}
