package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gopress/internal/store"
	"image"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxPortableContent = 16 << 20

type PortablePage struct {
	Kind, Slug, Title, Document, Cover string
	SEO                                store.SEO
	Translations                       map[string]store.Translation
}
type PortableImage struct {
	Name, Title, Alt, Caption string
	Data                      []byte
}
type PortableContent struct {
	Format  string
	Version int
	BaseURL string
	Pages   []PortablePage
	Images  []PortableImage
}

func portableImageName(name string) bool { return imagePath.MatchString("/uploads/" + name) }
func contentImages(p PortablePage, base string) (map[string]bool, error) {
	names := map[string]bool{}
	add := func(name string) error {
		if !portableImageName(name) {
			return ErrInvalid
		}
		names[name] = true
		return nil
	}
	if p.Cover != "" {
		if err := add(p.Cover); err != nil {
			return nil, err
		}
	}
	checkSEO := func(seo store.SEO) error {
		value := seo.SocialImage
		if strings.HasPrefix(value, base+"/uploads/") {
			value = strings.TrimPrefix(value, base)
		}
		if strings.HasPrefix(value, "/uploads/") {
			return add(strings.TrimPrefix(value, "/uploads/"))
		}
		return nil
	}
	if _, err := render(p.Document, add); err != nil {
		return nil, err
	}
	if err := checkSEO(p.SEO); err != nil {
		return nil, err
	}
	for _, tr := range p.Translations {
		if _, err := render(tr.Document, add); err != nil {
			return nil, err
		}
		if err := checkSEO(tr.SEO); err != nil {
			return nil, err
		}
	}
	return names, nil
}
func (v *Service) ExportContent(ctx context.Context, selection []store.Page) ([]byte, error) {
	if err := RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if len(selection) < 1 || len(selection) > store.ContentPageSize {
		return nil, ErrInvalid
	}
	v.mediaMu.RLock()
	defer v.mediaMu.RUnlock()
	site, err := v.Store.Site()
	if err != nil {
		return nil, err
	}
	bundle := PortableContent{Format: "gopress-content", Version: 1, BaseURL: site.BaseURL, Pages: []PortablePage{}, Images: []PortableImage{}}
	names := map[string]bool{}
	seen := map[int64]bool{}
	size := 0
	for _, selected := range selection {
		p, e := v.Store.Page(selected.ID)
		if e != nil {
			return nil, e
		}
		if p.Version != selected.Version || p.TrashedAt > 0 || seen[p.ID] {
			return nil, store.ErrConflict
		}
		seen[p.ID] = true
		item := PortablePage{Kind: p.Kind, Slug: p.Slug, Title: p.Title, Document: p.Document, Cover: p.Cover, SEO: p.SEO, Translations: p.Translations}
		size += len(p.Document)
		for _, tr := range p.Translations {
			size += len(tr.Document)
		}
		if size > 8<<20 {
			return nil, ErrInvalid
		}
		images, e := contentImages(item, site.BaseURL)
		if e != nil {
			return nil, e
		}
		for name := range images {
			names[name] = true
		}
		bundle.Pages = append(bundle.Pages, item)
	}
	if len(names) > 64 {
		return nil, ErrInvalid
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		f, e := v.Store.MediaFile(name)
		if e != nil {
			return nil, e
		}
		file, e := os.Open(filepath.Join(v.Store.Dir, "uploads", name))
		if e != nil {
			return nil, e
		}
		info, e := file.Stat()
		if e != nil || !info.Mode().IsRegular() {
			file.Close()
			return nil, ErrInvalid
		}
		data, e := io.ReadAll(io.LimitReader(file, MaxUpload+1))
		e = errors.Join(e, file.Close())
		if e != nil {
			return nil, e
		}
		size += len(data)*4/3 + 4
		if size > MaxPortableContent || len(data) > MaxUpload {
			return nil, ErrInvalid
		}
		bundle.Images = append(bundle.Images, PortableImage{Name: name, Title: f.Title, Alt: f.Alt, Caption: f.Caption, Data: data})
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPortableContent {
		return nil, ErrInvalid
	}
	return data, nil
}
func InspectContent(data []byte) (PortableContent, error) {
	var bundle PortableContent
	if len(data) == 0 || len(data) > MaxPortableContent || decodeStrict(string(data), &bundle) != nil || bundle.Format != "gopress-content" || bundle.Version != 1 || len(bundle.Pages) < 1 || len(bundle.Pages) > store.ContentPageSize || len(bundle.Images) > 64 {
		return bundle, ErrInvalid
	}
	if len(bundle.BaseURL) > 2048 || !SafeLink(bundle.BaseURL) {
		return bundle, ErrInvalid
	}
	images := map[string]bool{}
	imageBytes := 0
	for _, f := range bundle.Images {
		imageBytes += len(f.Data)
		if imageBytes > 8<<20 || !portableImageName(f.Name) || images[f.Name] || len(f.Title) > 200 || len(f.Alt) > 500 || len(f.Caption) > 1000 || !utf8.ValidString(f.Title+f.Alt+f.Caption) {
			return bundle, ErrInvalid
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(f.Data))
		if err != nil || (format != "png" && format != "jpeg") || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
			return bundle, ErrInvalid
		}
		images[f.Name] = true
	}
	used := map[string]bool{}
	for _, p := range bundle.Pages {
		if (p.Kind != "page" && p.Kind != "post") || len(p.Title) > 800 || !utf8.ValidString(p.Title) || !validContentPath(p.Slug) || len(p.Translations) > 20 {
			return bundle, ErrInvalid
		}
		names, err := contentImages(p, bundle.BaseURL)
		if err != nil {
			return bundle, err
		}
		for name := range names {
			if !images[name] {
				return bundle, ErrInvalid
			}
			used[name] = true
		}
	}
	if len(used) != len(images) {
		return bundle, ErrInvalid
	}
	return bundle, nil
}
func PortableSlug(prefix string, index int) string { return prefix + "-" + strconv.Itoa(index+1) }
func (v *Service) ImportContent(ctx context.Context, data []byte, prefix string) ([]store.Page, error) {
	if err := RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if len(prefix) > 60 || !validContentPath(prefix) || strings.Contains(prefix, "/") || store.ReservedContentPath(prefix) {
		return nil, ErrInvalid
	}
	bundle, err := InspectContent(data)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	uploaded := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, name := range uploaded {
				if e := v.DeleteMedia(context.Background(), name); e != nil {
					slog.Error("content import media cleanup failed")
				}
			}
		}
	}()
	for _, f := range bundle.Images {
		name, e := v.Upload(bytes.NewReader(f.Data), ctx)
		if e != nil {
			return nil, e
		}
		uploaded = append(uploaded, name)
		names[f.Name] = name
		meta, e := v.Store.MediaFile(name)
		if e != nil {
			return nil, e
		}
		meta.Title, meta.Alt, meta.Caption = f.Title, f.Alt, f.Caption
		if e = v.SaveMediaDetails(meta); e != nil {
			return nil, e
		}
	}
	// Resolve only exact attribute URLs; ordinary text and external links stay intact.
	links := map[string]string{}
	for i, p := range bundle.Pages {
		links["/"+p.Slug] = "/" + PortableSlug(prefix, i)
		links[bundle.BaseURL+"/"+p.Slug] = "/" + PortableSlug(prefix, i)
	}
	pages := make([]store.Page, 0, len(bundle.Pages))
	v.mediaMu.RLock()
	defer v.mediaMu.RUnlock()
	for i, p := range bundle.Pages {
		seo := p.SEO
		seo.Canonical = ""
		seo.SocialImage = portableURL(seo.SocialImage, bundle.BaseURL, names, links)
		doc, e := portableDocument(p.Document, bundle.BaseURL, names, links)
		if e != nil {
			return nil, e
		}
		for code, tr := range p.Translations {
			tr.Document, e = portableDocument(tr.Document, bundle.BaseURL, names, links)
			if e != nil {
				return nil, e
			}
			tr.SEO.Canonical = ""
			tr.SEO.SocialImage = portableURL(tr.SEO.SocialImage, bundle.BaseURL, names, links)
			p.Translations[code] = tr
		}
		draft := store.Page{Kind: p.Kind, Slug: PortableSlug(prefix, i), Title: p.Title, Document: doc, Cover: names[p.Cover], SEO: seo, Translations: p.Translations, Template: "page.html"}
		draft, e = v.preparePage(ctx, draft, false)
		if e != nil {
			return nil, e
		}
		pages = append(pages, draft)
	}
	saved, err := v.Store.ImportDrafts(ctx, pages)
	if err != nil {
		return nil, err
	}
	committed = true
	return saved, nil
}
func portableURL(value, base string, names, links map[string]string) string {
	if strings.HasPrefix(value, base+"/uploads/") {
		value = strings.TrimPrefix(value, base)
	}
	if name, ok := names[strings.TrimPrefix(value, "/uploads/")]; ok && strings.HasPrefix(value, "/uploads/") {
		return "/uploads/" + name
	}
	path, fragment, has := strings.Cut(value, "#")
	if target, ok := links[path]; ok {
		if has {
			target += "#" + fragment
		}
		return target
	}
	return value
}
func portableDocument(raw, base string, names, links map[string]string) (string, error) {
	var node Node
	if err := decodeStrict(raw, &node); err != nil {
		return "", err
	}
	var walk func(*Node) error
	attrs := func(raw json.RawMessage) (json.RawMessage, error) {
		if noAttrs(raw) {
			return raw, nil
		}
		var values map[string]any
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, ErrInvalid
		}
		for _, key := range []string{"src", "href"} {
			if value, ok := values[key].(string); ok {
				values[key] = portableURL(value, base, names, links)
			}
		}
		return json.Marshal(values)
	}
	walk = func(n *Node) error {
		var err error
		n.Attrs, err = attrs(n.Attrs)
		if err != nil {
			return err
		}
		for i := range n.Marks {
			n.Marks[i].Attrs, err = attrs(n.Marks[i].Attrs)
			if err != nil {
				return err
			}
		}
		for i := range n.Content {
			if err = walk(&n.Content[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(&node); err != nil {
		return "", err
	}
	data, err := json.Marshal(node)
	return string(data), err
}
