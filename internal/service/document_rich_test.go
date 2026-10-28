package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestToolbarFormattingContractAndCSP(t *testing.T) {
	raw, err := os.ReadFile("../../web/testdata/editor-format-document.json")
	if err != nil {
		t.Fatal(err)
	}
	body, err := Render(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`class="align-center"`, "<u>", "<s>", "<mark>", "<sup>", "<sub>"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("missing %s", fragment)
		}
	}
	if strings.Contains(body, " style=") {
		t.Fatal("alignment requires inline styles")
	}
	for _, raw := range []string{
		`{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"center;position:fixed"}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"highlight","attrs":{"color":"red"}}]}]}]}`,
	} {
		if _, err := Render(raw); err == nil {
			t.Fatal("accepted arbitrary styles")
		}
	}
}

func TestRichDocumentContract(t *testing.T) {
	raw, err := os.ReadFile("../../web/testdata/editor-rich-document.json")
	if err != nil {
		t.Fatal(err)
	}
	body, err := Render(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<ul><li><p>First item</p></li></ul>", `<ol start="2">`, "<blockquote>", `<pre><code class="language-go">`, "<hr>", `alt="Image &#34;description&#34;"`, "A &lt;caption&gt;", "<br>", "rel=\"noopener noreferrer\""} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("missing %s in %s", fragment, body)
		}
	}
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Page{Title: "Rich", Slug: "rich", Document: string(raw)}
	if _, err = v.Save(context.Background(), p, false); err == nil {
		t.Fatal("accepted nonexistent inline media")
	}
	if err = s.Media(strings.Repeat("a", 64)+".png", "image/png", 10); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Save(context.Background(), p, false); err != nil {
		t.Fatal(err)
	}
}
func TestRichRendererRejectsUnsafeStructures(t *testing.T) {
	for _, node := range []string{
		`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}`,
		`{"type":"image","attrs":{"src":"https://evil.test/a.png"}}`,
		`{"type":"image","attrs":{"src":"/uploads/../site.db"}}`,
		`{"type":"orderedList","attrs":{"start":-1},"content":[{"type":"listItem","content":[{"type":"paragraph"}]}]}`,
		`{"type":"listItem","content":[{"type":"paragraph"}]}`,
		`{"type":"codeBlock","attrs":{"language":"bad\" onclick=\"bad"},"content":[]}`,
		`{"type":"paragraph","content":[{"type":"image","attrs":{"src":"/uploads/a.png"}}]}`,
		`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"bold","attrs":{"onclick":"bad"}}]}]}`,
	} {
		if _, err := Render(`{"type":"doc","content":[` + node + `]}`); err == nil {
			t.Fatalf("accepted %s", node)
		}
	}
	nested := `{"type":"paragraph"}`
	for range 35 {
		nested = `{"type":"blockquote","content":[` + nested + `]}`
	}
	if _, err := Render(`{"type":"doc","content":[` + nested + `]}`); err == nil {
		t.Fatal("unbounded nesting")
	}
	for _, link := range []string{"https://example.com", "/contacts", "#anchor", "mailto:owner@example.com"} {
		if !SafeLink(link) {
			t.Fatal(link)
		}
	}
	for _, link := range []string{"javascript:alert(1)", "//evil.test", "/\\evil", "data:text/html,x", "https://user:pass@evil.test", "/a\nb"} {
		if SafeLink(link) {
			t.Fatal(link)
		}
	}
}
