package i18n

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestLanguagesAndCatalogCoverage(t *testing.T) {
	for _, language := range []string{"en", "pl", "de", "uk", "it", "es", "cs"} {
		ctx := WithLanguage(context.Background(), language)
		if T(ctx, " Настройки ") == " Настройки " || T(ctx, "My own title") != "My own title" {
			t.Fatal("translation or fallback", language)
		}
	}
	if T(context.Background(), "Настройки") != "Настройки" || Language(WithLanguage(context.Background(), "xx")) != "ru" {
		t.Fatal("default language")
	}
	for language, catalog := range catalogs {
		if !Supported(language) || Language(WithLanguage(context.Background(), language)) != language {
			t.Fatal("unsupported loaded catalog", language)
		}
		if len(catalog) != len(catalogs["en"]) {
			t.Fatal("catalog keys differ", language)
		}
		for key, value := range catalogs["en"] {
			translated := catalog[key]
			if value == "" || translated == "" {
				t.Fatal("incomplete catalog", language, key)
			}
			for _, pattern := range []string{`%[sw]`, `\{[a-z_]+\}`} {
				r := regexp.MustCompile(pattern)
				expected, actual := r.FindAllString(value, -1), r.FindAllString(translated, -1)
				if !reflect.DeepEqual(expected, actual) {
					t.Fatalf("%s: placeholders changed for %q: %v != %v", language, key, actual, expected)
				}
			}
		}
	}
	if Supported("xx") || Supported("") || Supported("ua") || Supported("cz") {
		t.Fatal("unsupported locale accepted")
	}
	err := filepath.WalkDir("..", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "T" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "i18n" {
				return true
			}
			literal, ok := call.Args[1].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			key = strings.TrimSpace(key)
			for _, language := range []string{"en", "pl", "de", "uk", "it", "es", "cs"} {
				if catalogs[language][key] == "" {
					t.Errorf("%s: missing %s translation for %q", path, language, key)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
