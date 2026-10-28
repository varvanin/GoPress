package httpui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func userBrowser(t *testing.T, app *App, email string) *browser {
	t.Helper()
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	status(t, b.request("GET", "/login", "", "", false), 200)
	status(t, b.request("POST", "/login", url.Values{"email": {email}, "password": {"a sufficiently long password"}}.Encode(), "application/x-www-form-urlencoded", true), 303)
	return b
}
func TestUserRolesObjectAccessAndBlocking(t *testing.T) {
	s, app, admin := seoApp(t)
	originalHandler := app.Echo.HTTPErrorHandler
	app.Echo.HTTPErrorHandler = func(c *echo.Context, err error) { t.Logf("HTTP error: %v", err); originalHandler(c, err) }
	ctx := context.Background()
	editorID, err := app.Service.SaveUser(ctx, store.User{Email: "editor@example.com", Role: "editor"}, "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := app.Service.SaveUser(ctx, store.User{Email: "author@example.com", Role: "author"}, "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	editor := userBrowser(t, app, "editor@example.com")
	author := userBrowser(t, app, "author@example.com")
	for _, path := range []string{"/admin/settings", "/admin/users", "/admin/themes", "/admin/forms", "/admin/menus"} {
		status(t, adminHTML(editor, "GET", path, nil, true, false, false), 403)
		status(t, adminHTML(author, "GET", path, nil, true, false, false), 403)
	}
	status(t, adminHTML(author, "GET", "/admin/pages", nil, true, false, false), 403)
	page, err := app.Service.Save(ctx, store.Page{Title: "Private page", Slug: "private-page", Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(author, "GET", fmt.Sprintf("/admin/pages/%d/edit", page.ID), nil, true, false, false), 403)
	u, _ := s.User(authorID)
	authorCtx := store.WithActor(ctx, u)
	post, err := app.Service.Save(authorCtx, store.Page{Kind: "post", Title: "Own draft", Slug: "own-draft", Document: document}, false)
	if err != nil || post.OwnerID != authorID {
		t.Fatal(post, err)
	}
	fields := url.Values{"kind": {"post"}, "id": {fmt.Sprint(post.ID)}, "version": {fmt.Sprint(post.Version)}, "title": {"Own draft"}, "slug": {"own-draft"}, "document": {document}, "intent": {"publish"}}
	status(t, adminHTML(author, "POST", "/admin/pages/save", fields, true, false, true), 403)
	status(t, adminHTML(author, "POST", fmt.Sprintf("/admin/pages/%d/publish", post.ID), nil, true, false, true), 403)
	fields.Set("intent", "save")
	status(t, adminHTML(author, "POST", "/admin/pages/save", fields, true, false, true), 303)
	list := adminHTML(author, "GET", "/admin/posts", nil, true, false, false)
	status(t, list, 200)
	if !strings.Contains(list.Body.String(), "Own draft") || strings.Contains(list.Body.String(), "Private page") {
		t.Fatal("author list leaks content")
	}
	status(t, adminHTML(editor, "GET", fmt.Sprintf("/admin/pages/%d/edit", post.ID), nil, true, false, false), 200)
	editorUser, _ := s.User(editorID)
	editorUser.Blocked = true
	if _, err = app.Service.SaveUser(ctx, editorUser, ""); err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(editor, "GET", "/admin/posts", nil, false, false, false), 303)
	last, _ := s.User(1)
	last.Blocked = true
	if _, err = app.Service.SaveUser(ctx, last, ""); err != store.ErrLastAdmin {
		t.Fatal("last administrator was blocked", err)
	}
	fields = url.Values{"id": {fmt.Sprint(page.ID)}, "version": {fmt.Sprint(page.Version)}, "kind": {"page"}, "title": {"Taken"}, "slug": {"private-page"}, "document": {document}, "intent": {"save"}}
	status(t, adminHTML(author, "POST", "/admin/pages/save", fields, true, false, true), 403)
	status(t, adminHTML(admin, "GET", "/admin/users", nil, false, false, false), 200)
	if _, err = app.Service.SaveUser(authorCtx, store.User{Email: "takeover@example.com", Role: "admin"}, "a sufficiently long password"); err != service.ErrForbidden {
		t.Fatal("author created administrator", err)
	}
}
