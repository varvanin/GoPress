package service

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopress/internal/store"
)

func cleanSEO(seo *store.SEO) error {
	seo.Title = strings.Join(strings.Fields(seo.Title), " ")
	seo.Description = strings.Join(strings.Fields(seo.Description), " ")
	if !utf8.ValidString(seo.Title+seo.Description) || utf8.RuneCountInString(seo.Title) > 200 || utf8.RuneCountInString(seo.Description) > 500 {
		return ErrInvalid
	}
	return cleanIndividualSEO(seo)
}

var themeIDPattern = regexp.MustCompile(`^theme:[a-z0-9][a-z0-9-]{0,63}$`)

func ValidTheme(id string) bool {
	return id == "journal" || id == "minimal" || id == "studio" || id == "custom" || themeIDPattern.MatchString(id)
}
func ValidateAppearance(a *store.Appearance) error {
	if a.Version < 0 || !ValidTheme(a.Theme) {
		return ErrInvalid
	}
	a.HomeTitle = strings.TrimSpace(a.HomeTitle)
	a.HomeDescription = strings.TrimSpace(a.HomeDescription)
	if !utf8.ValidString(a.HomeTitle+a.HomeDescription) || utf8.RuneCountInString(a.HomeTitle) > 200 || utf8.RuneCountInString(a.HomeDescription) > 1000 {
		return ErrInvalid
	}
	return cleanSEO(&a.SEO)
}
func (v *Service) SaveAppearance(ctx context.Context, a store.Appearance) error {
	if err := ValidateAppearance(&a); err != nil {
		return err
	}
	current, err := v.Store.Appearance()
	if err != nil {
		return err
	}
	a.SEOOptions = current.SEOOptions
	return v.Store.SaveAppearance(ctx, a)
}

var embedTextCode = regexp.MustCompile(`^\[(?:(?:form|faq) id=[1-9][0-9]{0,18}|module id="[a-z][a-z0-9-]{0,47}")\]$`)

// Excerpt reads the document schema, never strips tags from trusted HTML.
func documentText(raw string) string {
	var doc Node
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return ""
	}

	var out strings.Builder
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "paragraph" && len(n.Content) == 1 && n.Content[0].Type == "text" && len(n.Content[0].Marks) == 0 && embedTextCode.MatchString(n.Content[0].Text) {
			return
		}
		if n.Type == "text" {
			out.WriteString(n.Text)
		}
		for _, child := range n.Content {
			walk(child)
		}
		switch n.Type {
		case "paragraph", "heading", "blockquote", "listItem", "codeBlock", "hardBreak":
			out.WriteByte(' ')
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(out.String()), " ")
}
func Excerpt(raw string, limit int) string {
	text := []rune(documentText(raw))
	if len(text) > limit {
		return string(text[:limit]) + "…"
	}
	return string(text)
}

func (v *Service) SaveSiteSEO(ctx context.Context, version int64, seo store.SEO) error {
	if version < 0 {
		return ErrInvalid
	}
	if err := cleanSEO(&seo); err != nil {
		return err
	}
	value, err := v.Store.Appearance()
	if err != nil {
		return err
	}
	value.Version = version
	value.SEO = seo
	return v.Store.SaveAppearance(ctx, value)
}

func ReadingMinutes(raw string) int {
	words := len(strings.Fields(documentText(raw)))
	if words == 0 {
		return 0
	}
	return (words + 199) / 200
}
