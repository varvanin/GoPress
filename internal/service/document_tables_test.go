package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTableDocumentContractAndMalformedGrids(t *testing.T) {
	raw, err := os.ReadFile("../../web/testdata/editor-table-document.json")
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"<table><tbody><tr><th>", "&lt;script&gt;literal&lt;/script&gt;", "After table"} {
		if !strings.Contains(html, value) {
			t.Fatal(html)
		}
	}
	for _, damage := range []func(*Node){
		func(doc *Node) { doc.Content[0].Content[1].Content = doc.Content[0].Content[1].Content[:1] },
		func(doc *Node) {
			doc.Content[0].Content[0].Content[0].Attrs = json.RawMessage(`{"colspan":21,"rowspan":1}`)
		},
		func(doc *Node) {
			doc.Content[0].Content[0].Content[0].Attrs = json.RawMessage(`{"colspan":1,"rowspan":3}`)
		},
		func(doc *Node) {
			doc.Content[0].Content[0].Content[0].Attrs = json.RawMessage(`{"colspan":1,"rowspan":1,"onclick":"alert(1)"}`)
		},
	} {
		var doc Node
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		damage(&doc)
		invalid, _ := json.Marshal(doc)
		if _, err = Render(string(invalid)); err == nil {
			t.Fatal("malformed table accepted")
		}
	}
}
