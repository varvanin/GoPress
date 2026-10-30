package service

import (
	"os"
	"strings"
	"testing"
)

func TestSectionAnchorsRenderAndRejectUnsafeOrDuplicateNames(t *testing.T) {
	raw, err := os.ReadFile("../../web/testdata/editor-anchor-document.json")
	if err != nil {
		t.Fatal(err)
	}
	body, err := Render(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`href="#section-contacts"`, `<h2 class="align-center" id="section-contacts">`, `<p id="section-details">`} {
		if !strings.Contains(body, part) {
			t.Fatal(body)
		}
	}
	for _, name := range []string{"bad name", "1anchor", "<script>", "UPPER", strings.Repeat("a", 65), "details"} {
		document := strings.Replace(string(raw), `"anchor":"contacts"`, `"anchor":"`+name+`"`, 1)
		if _, err = Render(document); err == nil {
			t.Fatalf("accepted anchor %q", name)
		}
	}
	// An empty name removes the target without breaking old documents.
	if _, err = Render(strings.Replace(string(raw), `"anchor":"contacts"`, `"anchor":null`, 1)); err != nil {
		t.Fatal(err)
	}
}
