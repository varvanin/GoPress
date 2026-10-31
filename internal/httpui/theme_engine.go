package httpui

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"strconv"
	"strings"
	"text/template/parse"
	"time"

	"gopress/internal/service"
	"gopress/internal/store"
)

// Only missing files fall back; syntax and execution errors must remain visible.
func resolveThemePage(source fs.FS, prefix, requested string) (string, error) {
	candidates := []string{requested}
	switch requested {
	case "archive.html":
		candidates = append(candidates, "home.html")
	}
	if requested != "page.html" {
		candidates = append(candidates, "page.html")
	}
	for _, name := range candidates {
		if _, err := fs.Stat(source, prefix+name); err == nil {
			return name, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("page.html: обязательный запасной шаблон отсутствует")
}

func themeTemplates(source fs.FS, prefix, page string, zone *time.Location, sizes ...int) (*template.Template, error) {
	size := 12
	if len(sizes) > 0 {
		size = sizes[0]
	}
	if zone == nil {
		zone = time.UTC
	}
	t, err := template.New("theme").Funcs(template.FuncMap{
		"tr":          func(text string) string { return text },
		"contentURL":  publicContentURL,
		"languageURL": languageURL,
		"siteURL":     func(path string) string { return path },
		"gopressStep": func() (string, error) { return "", nil },
		"date":        func(at int64) string { return time.Unix(at, 0).In(zone).Format("02.01.2006") },
		"isoDate":     func(at int64) string { return time.Unix(at, 0).UTC().Format(time.RFC3339) },
		"add":         func(a, b int) int { return a + b }, "hasNext": func(page, total int) bool { return page*size < total },
		"cover": themeCover, "journalCover": journalCover, "postCount": postCount, "firstLetter": siteInitial,
		"asset": func(base, name string) (string, error) {
			if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:?#") {
				return "", fmt.Errorf("asset: недопустимый путь %q", name)
			}
			if base != "/assets" && !strings.HasPrefix(base, "/theme-assets/") && !strings.HasPrefix(base, "/builtin-assets/") && !strings.HasPrefix(base, "/admin/themes/editor/previews/") {
				return "", fmt.Errorf("asset: недопустимый адрес ресурсов")
			}
			segments := strings.Split(name, "/")
			for i, s := range segments {
				segments[i] = url.PathEscape(s)
			}
			return base + "/" + strings.Join(segments, "/"), nil
		},
	}).ParseFS(resources, "theme/seo.html")
	if err != nil {
		return nil, err
	}
	parse := func(name string) error {
		raw, e := fs.ReadFile(source, prefix+name)
		if e != nil {
			return fmt.Errorf("%s: %w", name, e)
		}
		_, e = t.New(name).Parse(string(raw))
		return e
	}
	alias := func(name, file string) error {
		if existing := t.Lookup(name); existing != nil && existing.Tree != nil && existing.Tree.ParseName == file {
			return nil
		}
		_, e := t.New(name).Parse(`{{template "` + file + `" .}}`)
		return e
	}
	if err = parse("layout.html"); err != nil {
		return nil, err
	}
	if err = alias("layout", "layout.html"); err != nil {
		return nil, err
	}
	for _, name := range []string{"header.html", "footer.html"} {
		if _, e := fs.Stat(source, prefix+name); errors.Is(e, fs.ErrNotExist) {
			continue
		} else if e != nil {
			return nil, e
		}
		if err = parse(name); err != nil {
			return nil, err
		}
		if err = alias(strings.TrimSuffix(name, ".html"), name); err != nil {
			return nil, err
		}
	}
	var parts []string
	root := prefix + "parts"
	if _, e := fs.Stat(source, root); e == nil {
		err = fs.WalkDir(source, root, func(name string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && strings.HasSuffix(name, ".html") {
				parts = append(parts, strings.TrimPrefix(name, prefix))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	for _, name := range parts {
		if err = parse(name); err != nil {
			return nil, err
		}
		if err = alias(strings.TrimSuffix(name, ".html"), name); err != nil {
			return nil, err
		}
	}
	page, err = resolveThemePage(source, prefix, page)
	if err != nil {
		return nil, err
	}
	if err = parse(page); err != nil {
		return nil, err
	}
	if err = alias("body", page); err != nil {
		return nil, err
	}
	addThemeSteps(t)
	return t, nil
}

func validateThemeFiles(source fs.FS, prefix string, appearance store.Appearance) error {
	root := strings.TrimSuffix(prefix, "/")
	if root == "" {
		root = "."
	}
	entries, err := fs.ReadDir(source, root)
	if err != nil {
		return err
	}
	pages := []string{"home.html", "single.html", "archive.html", "search.html", "404.html", "page.html"}
	for _, entry := range entries {
		if !entry.IsDir() && service.ValidPageTemplate(entry.Name()) {
			pages = append(pages, entry.Name())
		}
	}
	seen := map[string]bool{}
	for _, page := range pages {
		if seen[page] {
			continue
		}
		seen[page] = true
		resolved, e := resolveThemePage(source, prefix, page)
		if e != nil {
			return e
		}
		t, e := themeTemplates(source, prefix, page, time.UTC)
		if e != nil {
			return fmt.Errorf("%s: %w", page, e)
		}
		for _, populated := range []bool{false, true} {
			data := ThemeData{Appearance: appearance, Title: "Предпросмотр", Site: ThemeSite{Title: "GoPress", BaseURL: "https://example.com", Timezone: "UTC"}, ThemeAssets: "/theme-assets/preview", Kind: "post", Slug: "example", Home: page == "home.html", Archive: page == "archive.html", NotFound: page == "404.html", Template: resolved, Body: "<p>Текст материала</p>", BlogPage: 1, BlogPageSize: 12, Year: 2026}
			if page != "single.html" {
				data.Kind = "page"
			}
			data.Preview = populated
			data.HomeIsPage = populated
			if populated {
				post := store.Page{Title: "Запись", Slug: "post", PublishedAt: 1, Cover: "example.png", SEO: store.SEO{Description: "Описание"}}
				term := store.Term{ID: 1, Name: "Категория", Kind: "category"}
				views := postViews([]store.Page{post}, []store.Term{term})
				data.Cover = post.Cover
				data.Description = "Описание"
				data.PublishedAt = 1
				data.ReadingMinutes = 1
				data.BlogTotal = 24
				data.Pages = []store.Page{post}
				data.Posts = []store.Page{post}
				data.Terms = []store.Term{term}
				data.Categories = data.Terms
				data.PostTerms = data.Terms
				data.ActiveTerm = &term
				data.Feed = views
				data.Spotlight = views
				data.Related = views
				data.Featured = &views[0]
				data.HeaderMenuConfigured = true
				data.HeaderMenu = []MenuLink{{Label: "Главная", URL: "/", Children: []MenuLink{{Label: "Страница", URL: "/page"}}}}
				data.FooterMenu = data.HeaderMenu
				data.Pagination = themePagination("/", url.Values{}, 1, 24, 12)
			}
			if page == "search.html" {
				data.Search = &PublicSearchData{Query: "Пример", Page: 1}
				if populated {
					data.Search.Total = 1
					data.Search.Results = []store.SearchResult{{Title: "Запись", Slug: "post", Snippet: "Пример текста"}}
				}
			}
			out := themeOutput{limit: 2 << 20}
			if e = executeTheme(t, &out, data); e != nil {
				return fmt.Errorf("%s: %w", page, e)
			}
		}
	}
	return nil
}

type ThemePagination struct {
	Current, TotalPages, Total int
	PreviousURL, NextURL       string
}

func themePagination(route string, query url.Values, current, total, size int) ThemePagination {
	p := ThemePagination{Current: current, Total: total, TotalPages: (total + size - 1) / size}
	link := func(page int) string {
		values := url.Values{}
		for _, key := range []string{"q", "term"} {
			if value := query.Get(key); value != "" {
				values.Set(key, value)
			}
		}
		if page > 1 {
			values.Set("page", strconv.Itoa(page))
		}
		if encoded := values.Encode(); encoded != "" {
			return route + "?" + encoded
		}
		return route
	}
	if current > 1 {
		p.PreviousURL = link(current - 1)
	}
	if current < p.TotalPages {
		p.NextURL = link(current + 1)
	}
	return p
}

// Stop execution before buffering oversized output or running excessive empty loops.
type themeOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (w *themeOutput) Write(p []byte) (int, error) {
	limit := w.limit
	if limit == 0 {
		limit = 8 << 20
	}
	if w.Len()+len(p) > limit {
		return 0, fmt.Errorf("template output exceeds %d MiB", limit>>20)
	}
	return w.buffer.Write(p)
}
func (w *themeOutput) Len() int       { return w.buffer.Len() }
func (w *themeOutput) String() string { return w.buffer.String() }

func executeTheme(t *template.Template, w *themeOutput, data ThemeData) error {
	steps := 100000
	t.Funcs(template.FuncMap{"gopressStep": func() (string, error) {
		steps--
		if steps < 0 {
			return "", fmt.Errorf("template execution exceeds 100000 steps")
		}
		return "", nil
	}})
	return t.ExecuteTemplate(w, "layout", data)
}
func addThemeSteps(t *template.Template) {
	var action *parse.ActionNode
	var visit func(*parse.ListNode)
	visit = func(list *parse.ListNode) {
		if list == nil {
			return
		}
		for _, node := range list.Nodes {
			switch n := node.(type) {
			case *parse.RangeNode:
				visit(n.List)
				visit(n.ElseList)
			case *parse.IfNode:
				visit(n.List)
				visit(n.ElseList)
			case *parse.WithNode:
				visit(n.List)
				visit(n.ElseList)
			}
		}
		tick := *action
		list.Nodes = append([]parse.Node{&tick}, list.Nodes...)
	}
	for _, part := range t.Templates() {
		if part.Tree != nil {
			step := template.Must(template.New(part.Tree.ParseName).Funcs(template.FuncMap{"gopressStep": func() string { return "" }}).Parse("{{$gopressBudget := gopressStep}}"))
			action = step.Tree.Root.Nodes[0].(*parse.ActionNode)
			visit(part.Tree.Root)
		}
	}
}
