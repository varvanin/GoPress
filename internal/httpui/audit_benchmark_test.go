package httpui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopress/internal/service"
	"gopress/internal/store"
)

func BenchmarkBuiltinTemplateParse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := themeTemplates(resources, "theme/", "page.html", time.UTC); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkTemplateEditorSelection(b *testing.B) {
	dir := b.TempDir()
	root := filepath.Join(dir, "theme")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		b.Fatal(err)
	}
	for name, data := range map[string][]byte{"layout.html": []byte("layout"), "page.html": []byte("page"), "assets/image.webp": make([]byte, 4<<20)} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			b.Fatal(err)
		}
	}
	app := &App{Service: &service.Service{Store: &store.Store{Dir: dir}}}
	for _, selected := range []string{"", "page.html"} {
		b.Run(fmt.Sprintf("selected=%q", selected), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := app.templateFiles("custom", selected); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
