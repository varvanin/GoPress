package httpui

import (
	"context"
	"gopress/internal/store"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSectionAnchorsFollowPublicationSnapshots(t *testing.T) {
	_, app, b := seoApp(t)
	raw, err := os.ReadFile("../../web/testdata/editor-anchor-document.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := app.Service.Save(context.Background(), store.Page{Title: "Landing", Slug: "landing", Document: string(raw)}, true)
	if err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/landing", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), `id="section-contacts"`) {
		t.Fatal("anchor not published")
	}
	p.Document = strings.ReplaceAll(p.Document, "contacts", "new-contact")
	if _, err = app.Service.Save(context.Background(), p, false); err != nil {
		t.Fatal(err)
	}
	response = b.request("GET", "/landing", "", "", false)
	if strings.Contains(response.Body.String(), `id="section-new-contact"`) || !strings.Contains(response.Body.String(), `id="section-contacts"`) {
		t.Fatal("draft anchor leaked")
	}
	response = b.request("GET", "/admin/pages/"+strconv.FormatInt(p.ID, 10)+"/preview", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), `id="section-new-contact"`) {
		t.Fatal("preview anchor missing")
	}
}
