package httpui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/service"
	"gopress/internal/store"
)

func TestEditorialTheme(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	if err := app.Service.Install(ctx, app.Service.InstallToken, store.Site{Title: "Форма и смысл", BaseURL: "https://example.com", Timezone: "Europe/Warsaw"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	b := &browser{app: app, cookies: map[string]*http.Cookie{}}
	appearance, err := s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = "journal"
	appearance.HomeTitle = "Замечать больше. Делиться главным."
	appearance.HomeDescription = "Журнал о дизайне, культуре и внимании к повседневному. Идеи, которые стоит сохранить, и истории, к которым хочется вернуться."
	if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Дизайн", "Культура", "Практика"} {
		if err = app.Service.SaveTerm(ctx, store.Term{Kind: "category", Name: name}, false); err != nil {
			t.Fatal(err)
		}
	}
	terms, err := s.Terms(false)
	if err != nil {
		t.Fatal(err)
	}
	titles := []string{"Красота простых вещей", "Пространство для новых идей", "Как сохранить любопытство", "Письма из тихого города", "Вещи, которые остаются", "Новый взгляд на привычное", "Искусство делать паузу", "Хороший текст начинается с внимания", "Город, который учит смотреть", "Меньше шума. Больше смысла."}
	excerpts := []string{"Почему самые полезные решения часто оказываются самыми простыми.", "Как окружающая среда помогает думать свободнее и находить неожиданные решения.", "О небольших привычках, которые делают каждый день интереснее.", "Наблюдения за архитектурой, светом и неспешным ритмом улиц.", "О предметах с характером и осознанном отношении к выбору.", "Иногда достаточно сменить маршрут, чтобы увидеть больше.", "Почему свободное время — важная часть любого творческого процесса.", "От первой заметки до ясной истории: несколько принципов работы со словами.", "Детали, которые мы проходим каждый день, заслуживают отдельной истории.", "Разбираемся, как создавать вещи с характером и оставлять место для главного."}
	rich := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Хорошая история начинается с простого наблюдения. Стоит остановиться, посмотреть вокруг и дать себе немного времени. Так привычные вещи обретают новый смысл."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Оставить место для главного"}]},{"type":"paragraph","content":[{"type":"text","text":"Мы привыкли добавлять: новые детали, дополнительные возможности, ещё один пункт в списке. Но ясность часто появляется в тот момент, когда мы начинаем убирать лишнее."}]},{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"Внимание — самый ценный материал, с которым мы работаем."}]}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Небольшая практика"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Сформулируйте одну главную мысль."}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Найдите деталь, которая делает историю вашей."}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Уберите всё, что отвлекает от смысла."}]}]}]},{"type":"paragraph","content":[{"type":"text","text":"Не обязательно менять всё сразу. Начните с малого: одной страницы, одного предмета, одного разговора. Внимание к деталям постепенно становится привычкой — и меняет то, как мы видим мир."}]}]}`
	if _, err = app.Service.Save(ctx, store.Page{Title: "О журнале", Slug: "about", Document: rich}, true); err != nil {
		t.Fatal(err)
	}
	paths := []string{"/", "/missing", "/rss.xml", "/about", "/search", "/search?q=внимание"}
	var last store.Page
	for i, title := range titles {
		last, err = app.Service.Save(ctx, store.Page{Kind: "post", Title: title, Slug: fmt.Sprintf("story-%d", i), Document: rich, SEO: store.SEO{Description: excerpts[i]}, TermIDs: []int64{terms[i%len(terms)].ID}}, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.ExecContext(ctx, "UPDATE pages SET published_at=? WHERE id=?", 1791878400+int64(i)*86400, last.ID); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, "/"+last.Slug)
	}
	last.SEO.Description = "PRIVATE DRAFT EXCERPT"
	last.TermIDs = nil
	if _, err = app.Service.Save(ctx, last, false); err != nil {
		t.Fatal(err)
	}
	posts, _, err := s.PublicPosts(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 10 || posts[0].Document != "" || posts[0].SEO.Description != excerpts[9] || len(posts[0].TermIDs) != 1 {
		t.Fatal("public list must use lightweight published metadata")
	}
	home := b.request("GET", "/", "", "", false)
	status(t, home, 200)
	html := home.Body.String()
	if strings.Contains(html, "PRIVATE DRAFT") || strings.Contains(html, "<script") {
		t.Fatal("draft data or unnecessary scripts in public theme")
	}
	for _, title := range titles {
		if strings.Count(html, ">"+title+"</a>") != 1 {
			t.Errorf("story %q must appear once", title)
		}
	}
	article := b.request("GET", "/story-9", "", "", false)
	status(t, article, 200)
	for _, marker := range []string{"мин чтения", "Ещё почитать", "<blockquote>", "article-content", "Подписаться через RSS"} {
		if !strings.Contains(article.Body.String(), marker) {
			t.Errorf("article missing %s", marker)
		}
	}
	for _, term := range terms {
		path := fmt.Sprintf("/?term=%d", term.ID)
		paths = append(paths, path)
		response := b.request("GET", path, "", "", false)
		status(t, response, 200)
		if strings.Contains(response.Body.String(), `class="featured-story"`) {
			t.Fatal("filtered list must not repeat the lead layout")
		}
	}
	for _, asset := range []string{"journal-cover-1.webp", "journal-cover-2.webp", "journal-cover-3.webp", "public.css", "public-common.css", "public-minimal.css", "public-studio.css", "editorial-cover-1.svg", "editorial-cover-2.svg", "editorial-cover-3.svg"} {
		status(t, b.request("GET", "/assets/"+asset, "", "", false), 200)
	}
	// Export the same published fixture through each requested template.
	var demoRewrite *strings.Replacer
	if dir := os.Getenv("GOPRESS_THEME_PREVIEW"); dir != "" {
		demoRewrite = exportThemeFixture(t, b, dir, paths, []string{"journal-cover-1.webp", "journal-cover-2.webp", "journal-cover-3.webp", "public.css", "public-common.css", "public-minimal.css", "public-studio.css", "editorial-cover-1.svg", "editorial-cover-2.svg", "editorial-cover-3.svg"})
	}

	appearance, err = s.Appearance()
	if err != nil {
		t.Fatal(err)
	}
	appearance.Theme = "minimal"
	if err = app.Service.SaveAppearance(ctx, appearance); err != nil {
		t.Fatal(err)
	}
	minimal := b.request("GET", "/", "", "", false)
	status(t, minimal, 200)
	if dir := os.Getenv("GOPRESS_THEME_PREVIEW"); dir != "" {
		html := demoRewrite.Replace(minimal.Body.String())
		if err = os.WriteFile(filepath.Join(dir, "minimal.html"), []byte(html), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if dir := os.Getenv("GOPRESS_MINIMAL_PREVIEW"); dir != "" {
		exportThemeFixture(t, b, dir, paths, []string{"public-minimal.css"})
	}

	if strings.Contains(minimal.Body.String(), `class="featured-story"`) || strings.Count(minimal.Body.String(), `class="post-card"`) != 10 {
		t.Fatal("minimal theme must keep every post in its list")
	}
}

func TestEditorialHelpers(t *testing.T) {
	for n, want := range map[int]string{0: "0 записей", 1: "1 запись", 2: "2 записи", 11: "11 записей", 21: "21 запись", 114: "114 записей"} {
		if got := postCount(n); got != want {
			t.Errorf("postCount(%d)=%q", n, got)
		}
	}
	if siteInitial("  Журнал ") != "Ж" || siteInitial("") != "G" {
		t.Fatal("brand initial")
	}
	if themeCover("story", "photo.jpg") != "/uploads/photo.jpg" || !strings.HasPrefix(themeCover("story", ""), "/assets/editorial-cover-") {
		t.Fatal("cover selection")
	}
	for _, n := range []int{0, 1, 200, 201} {
		raw := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, strings.Repeat("word ", n))
		if got := service.ReadingMinutes(raw); got != (n+199)/200 {
			t.Errorf("reading %d words = %d", n, got)
		}
	}
}
