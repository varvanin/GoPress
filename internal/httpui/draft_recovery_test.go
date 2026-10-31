package httpui

import (
	"context"
	"fmt"
	"gopress/internal/store"
	"net/url"
	"strings"
	"testing"
)

func TestLocalRecoveryDoesNotSaveAndRejectsStaleOrForeignDrafts(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	p, err := app.Service.Save(ctx, store.Page{Kind: "post", Title: "Server draft", Slug: "recovery", Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	fields := url.Values{"id": {fmt.Sprint(p.ID)}, "version": {fmt.Sprint(p.Version)}, "kind": {"post"}, "title": {"Local draft"}, "slug": {"recovery"}, "document": {document}, "source_language": {"ru"}, "content_language": {"original"}}
	payload := url.Values{"id": fields["id"], "version": fields["version"], "snapshot": {fields.Encode()}}
	status(t, adminHTML(b, "POST", "/admin/pages/recover", payload, false, false, false), 403)
	response := adminHTML(b, "POST", "/admin/pages/recover", payload, false, false, true)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Local draft") || !strings.Contains(response.Body.String(), `data-recovered="true"`) {
		t.Fatal("recovered editor missing draft or dirty guard")
	}
	unchanged, err := s.Page(p.ID)
	if err != nil || unchanged.Title != "Server draft" || unchanged.Version != p.Version {
		t.Fatal("recovery wrote data", unchanged, err)
	}
	if _, err = app.Service.SaveUser(ctx, store.User{Email: "recovery-author@example.test", Role: "author"}, "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	author := userBrowser(t, app, "recovery-author@example.test")
	status(t, adminHTML(author, "POST", "/admin/pages/recover", payload, false, false, true), 403)
	p.Title = "Newer server draft"
	if _, err = app.Service.Save(ctx, p, false); err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(b, "POST", "/admin/pages/recover", payload, false, false, true), 409)
}
