package httpui

import (
	"context"
	"fmt"
	"gopress/internal/store"
	"net/url"
	"testing"
)

func TestReviewCannotPublishChangedOrReturnedDraft(t *testing.T) {
	s, app, b := seoApp(t)
	ctx := context.Background()
	id, err := app.Service.SaveUser(ctx, store.User{Email: "author@example.test", Role: "author"}, "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	author, _ := s.User(id)
	actor := store.WithActor(ctx, author)
	p, err := app.Service.Save(actor, store.Page{Kind: "post", Title: "Review", Slug: "review-post", Document: document}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = app.Service.RequestReview(actor, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	values := url.Values{"page_id": {fmt.Sprint(p.ID)}, "version": {fmt.Sprint(p.Version)}, "action": {"publish"}}
	a := userBrowser(t, app, author.Email)
	status(t, adminHTML(a, "POST", "/admin/posts/review", values, false, false, true), 403)
	status(t, adminHTML(b, "POST", "/admin/posts/review", values, false, false, false), 403)
	old := p.Version
	p.Title = "New version"
	p, err = app.Service.Save(actor, p, false)
	if err != nil {
		t.Fatal(err)
	}
	status(t, adminHTML(b, "POST", "/admin/posts/review", values, false, false, true), 409)
	values.Set("action", "return")
	values.Set("comment", "Please check the title")
	status(t, adminHTML(b, "POST", "/admin/posts/review", values, false, false, true), 303)
	values.Set("action", "publish")
	values.Set("version", fmt.Sprint(old))
	status(t, adminHTML(b, "POST", "/admin/posts/review", values, false, false, true), 409)
	if err = app.Service.RequestReview(actor, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	values.Set("version", fmt.Sprint(p.Version))
	status(t, adminHTML(b, "POST", "/admin/posts/review", values, false, false, true), 303)
	published, err := s.Public(p.Slug)
	if err != nil || published.Title != "New version" {
		t.Fatal(published, err)
	}
	items, err := s.ContentReviews(0)
	if err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}
