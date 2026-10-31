package httpui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestResponsiveImagesAndThumbnailRoute(t *testing.T) {
	_, app, b := seoApp(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1200, 600))); err != nil {
		t.Fatal(err)
	}
	name, err := app.Service.Upload(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	source := "/uploads/" + name
	doc := `{"type":"doc","content":[{"type":"image","attrs":{"src":"` + source + `","alt":"A & B","caption":""}}]}`
	if _, err = app.Service.Save(context.Background(), store.Page{Title: "Responsive", Slug: "responsive", Document: doc, Cover: name}, true); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/responsive", "", "", false)
	status(t, response, 200)
	for _, want := range []string{`?w=320 320w`, `?w=960 960w`, source + ` 1200w`, `width="1200"`, `height="600"`, `A &amp; B`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatal("missing image metadata", want)
		}
	}
	thumb := b.request("GET", source+"?w=320", "", "", false)
	status(t, thumb, 200)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb.Body.Bytes()))
	if err != nil || cfg.Width != 320 || cfg.Height != 160 {
		t.Fatal(cfg, err)
	}
	original := b.request("GET", source, "", "", false)
	status(t, original, 200)
	cfg, _, err = image.DecodeConfig(bytes.NewReader(original.Body.Bytes()))
	if err != nil || cfg.Width != 1200 {
		t.Fatal(cfg, err)
	}
	status(t, b.request("GET", source+"?w=999999", "", "", false), 400)
	raw := `<img src="` + source + `" srcset="custom.png 1x" sizes="50vw" width="10"><script>const a='<img src="` + source + `">';</script>`
	rendered, err := app.responsiveImages(raw)
	if err != nil || !strings.Contains(rendered, `srcset="custom.png 1x"`) || !strings.Contains(rendered, `sizes="50vw"`) {
		t.Fatal(rendered, err)
	}
	if !strings.Contains(rendered, `const a='<img src="`+source+`">'`) {
		t.Fatal("rewrote script data")
	}
}
