package httpui

import (
	"gopress/internal/store"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode/utf8"
)

type PostView struct {
	store.Page
	Category *store.Term
}

func postViews(posts []store.Page, terms []store.Term) []PostView {
	categories := map[int64]store.Term{}
	for _, term := range terms {
		if term.Kind == "category" {
			categories[term.ID] = term
		}
	}
	result := make([]PostView, 0, len(posts))
	for _, post := range posts {
		view := PostView{Page: post}
		for _, id := range post.TermIDs {
			if term, ok := categories[id]; ok {
				view.Category = &term
				break
			}
		}
		result = append(result, view)
	}
	return result
}
func themeCover(slug, name string) string {
	if name != "" {
		return "/uploads/" + name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return "/assets/editorial-cover-" + strconv.Itoa(int(h.Sum32()%3)+1) + ".svg"
}
func postCount(n int) string {
	word := "записей"
	if n%100 < 11 || n%100 > 14 {
		switch n % 10 {
		case 1:
			word = "запись"
		case 2, 3, 4:
			word = "записи"
		}
	}
	return strconv.Itoa(n) + " " + word
}
func siteInitial(title string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(title))
	if r == utf8.RuneError || r == 0 {
		return "G"
	}
	return strings.ToUpper(string(r))
}

func journalCover(slug, name string) string {
	if name != "" {
		return "/uploads/" + name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return "/assets/journal-cover-" + strconv.Itoa(int(h.Sum32()%3)+1) + ".webp"
}
