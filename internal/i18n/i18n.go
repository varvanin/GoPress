package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
)

//go:embed locales/*.json
var files embed.FS
var catalogs = map[string]map[string]string{}
var sources = map[string]string{}

type languageKey struct{}

func init() {
	for _, language := range []string{"en", "pl", "de", "uk", "it", "es", "cs"} {
		raw, err := files.ReadFile("locales/" + language + ".json")
		if err != nil {
			panic(err)
		}
		var catalog map[string]string
		if err = json.Unmarshal(raw, &catalog); err != nil {
			panic(err)
		}
		catalogs[language] = catalog
		for source, translation := range catalog {
			sources[translation] = source
		}
	}
}
func WithLanguage(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, languageKey{}, language)
}
func Language(ctx context.Context) string {
	if language, _ := ctx.Value(languageKey{}).(string); Supported(language) {
		return language
	}
	return "ru"
}
func T(ctx context.Context, text string) string {
	key := strings.TrimSpace(text)
	translated, ok := catalogs[Language(ctx)][key]
	if !ok {
		return text
	}
	start := strings.Index(text, key)
	return text[:start] + translated + text[start+len(key):]
}
func Source(text string) string {
	if source, ok := sources[text]; ok {
		return source
	}
	return text
}
func Catalog(language string) map[string]string { return catalogs[language] }

func Supported(language string) bool {
	if language == "ru" {
		return true
	}
	_, ok := catalogs[language]
	return ok
}
