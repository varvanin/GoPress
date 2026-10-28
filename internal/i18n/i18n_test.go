package i18n

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLanguagesAndCatalogCoverage(t *testing.T) {
	for _, language := range []string{"en", "pl"} {
		ctx := WithLanguage(context.Background(), language)
		if T(ctx, " Настройки ") == " Настройки " || T(ctx, "My own title") != "My own title" {
			t.Fatal("translation or fallback", language)
		}
	}
	if T(context.Background(), "Настройки") != "Настройки" || Language(WithLanguage(context.Background(), "xx")) != "ru" {
		t.Fatal("default language")
	}
	for key, value := range catalogs["en"] {
		if value == "" || catalogs["pl"][key] == "" {
			t.Fatal("incomplete catalog", key)
		}
	}
	if len(catalogs["en"]) != len(catalogs["pl"]) {
		t.Fatal("catalog keys differ")
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
			for _, language := range []string{"en", "pl"} {
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
