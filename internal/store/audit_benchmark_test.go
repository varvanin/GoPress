package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkModuleReadProjection(b *testing.B) {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	if err = s.InstallModulePackage(context.Background(), ModulePackage{ID: "test", Manifest: `{}`, Template: "<p>Block</p>", Settings: `{}`, Screenshot: make([]byte, 2<<20)}); err != nil {
		b.Fatal(err)
	}
	for _, full := range []bool{true, false} {
		b.Run(fmt.Sprint("full=", full), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if full {
					_, err = s.ModulePackage("test")
				} else {
					_, err = s.ModuleContent("test")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkGalleryMediaLookup(b *testing.B) {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	names := []string{}
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("image-%d.png", i)
		if err = s.Media(name, "image/png", 1); err != nil {
			b.Fatal(err)
		}
		names = append(names, name)
	}
	b.Run("individual", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, name := range names {
				if _, err = s.MediaFile(name); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err = s.MediaFiles(names); err != nil {
				b.Fatal(err)
			}
		}
	})
}
func BenchmarkMediaReferenceHistory(b *testing.B) {
	for _, count := range []int{0, 50, 500} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s, err := Open(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			if err = s.Media("unused.png", "image/png", 1); err != nil {
				b.Fatal(err)
			}
			doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("a", 32<<10) + `"}]}]}`
			p, err := s.Save(context.Background(), Page{Title: "Page", Slug: "page", Document: doc}, false)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < count; i++ {
				p, err = s.Save(context.Background(), p, false)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				used, err := s.MediaUsed("unused.png")
				if err != nil || used {
					b.Fatal(used, err)
				}
			}

		})
	}
}
