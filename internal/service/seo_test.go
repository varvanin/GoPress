package service

import (
	"gopress/internal/store"
	"strings"
	"testing"
)

func TestSEONormalizationAndExcerpt(t *testing.T) {
	seo := store.SEO{Title: "  Заголовок\n  страницы ", Description: " Строка\nописания "}
	if err := cleanSEO(&seo); err != nil || seo.Title != "Заголовок страницы" || seo.Description != "Строка описания" {
		t.Fatal(seo, err)
	}
	seo.Title = strings.Repeat("я", 200)
	if err := cleanSEO(&seo); err != nil {
		t.Fatal(err)
	}
	seo.Title += "я"
	if cleanSEO(&seo) == nil {
		t.Fatal("oversized title accepted")
	}
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Привет ","marks":[{"type":"bold"}]},{"type":"text","text":"мир"}]},{"type":"paragraph","content":[{"type":"text","text":"<script> текст"}]}]}`
	if got := Excerpt(doc, 12); got != "Привет мир <…" {
		t.Fatal(got)
	}
	if got := Excerpt(doc, 100); got != "Привет мир <script> текст" {
		t.Fatal(got)
	}
	a := store.Appearance{Theme: "../../outside"}
	if ValidateAppearance(&a) == nil {
		t.Fatal("path accepted")
	}
}
