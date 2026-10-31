package httpui

import (
	"context"
	"gopress/internal/store"
	"strings"
	"testing"
)

func TestSetupChecklistUsesActualPublicationState(t *testing.T) {
	_, app, b := seoApp(t)
	response := b.request("GET", "/admin/setup", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "Не завершено") {
		t.Fatal("empty installation marked complete")
	}
	p, err := app.Service.Save(context.Background(), store.Page{Kind: "post", Title: "Published", Slug: "setup-post", Document: document}, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = p
	response = b.request("GET", "/admin/setup", "", "", false)
	status(t, response, 200)
	if strings.Count(response.Body.String(), "Не завершено") != 1 {
		t.Fatal("publication state not reflected")
	}
}
