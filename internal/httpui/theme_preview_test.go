package httpui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A separate static fixture keeps design review away from the working database.
func exportThemeFixture(t *testing.T, b *browser, dir string, paths, assets []string) *strings.Replacer {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	filename := func(path string) string {
		switch path {
		case "/":
			return "index.html"
		case "/rss.xml":
			return "rss.xml"
		}
		return strings.NewReplacer("/", "", "?term=", "category-", "?q=", "-query-").Replace(path) + ".html"
	}
	pairs := []string{`"/assets/`, `"assets/`, `"/#journal"`, `"index.html#journal"`, `"/#entries"`, `"index.html#entries"`}
	for _, path := range paths {
		pairs = append(pairs, `href="`+path+`"`, `href="`+filename(path)+`"`)
		for _, anchor := range []string{"journal", "entries"} {
			pairs = append(pairs, `href="`+path+`#`+anchor+`"`, `href="`+filename(path)+`#`+anchor+`"`)
		}
	}
	rewrite := strings.NewReplacer(pairs...)
	for _, path := range paths {
		response := b.request("GET", path, "", "", false)
		want := 200
		if path == "/missing" {
			want = 404
		}
		status(t, response, want)
		if err := os.WriteFile(filepath.Join(dir, filename(path)), []byte(rewrite.Replace(response.Body.String())), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range assets {
		data, err := resources.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "assets", name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return rewrite
}
