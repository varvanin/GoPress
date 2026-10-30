package httpui

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestContentBlocksRejectExcessBeforeRendering(t *testing.T) {
	for _, test := range []struct {
		pattern *regexp.Regexp
		code    string
	}{
		{faqShortcode, "<p>[faq id=1]</p>"}, {formShortcode, "<p>[form id=1]</p>"},
		{moduleShortcode, `<p>[module id="test"]</p>`}, {collectionShortcode, "<p>[gallery id=1]</p>"},
	} {
		calls := 0
		_, err := replaceContentBlocks(strings.Repeat(test.code, 51), test.pattern, func(string) (string, error) { calls++; return "block", nil })
		if err == nil || calls != 0 {
			t.Fatalf("%s: excessive blocks rendered: %d, %v", test.pattern, calls, err)
		}
	}
}
func TestContentBlocksStopAtOutputBudgetOrRenderError(t *testing.T) {
	calls := 0
	_, err := replaceContentBlocks(strings.Repeat("<p>[faq id=1]</p>", 40), faqShortcode, func(string) (string, error) { calls++; return strings.Repeat("a", 256<<10), nil })
	if err == nil || calls >= 40 {
		t.Fatal("unbounded expansion", calls, err)
	}
	expected := errors.New("render failed")
	calls = 0
	_, err = replaceContentBlocks(strings.Repeat("<p>[faq id=1]</p>", 3), faqShortcode, func(string) (string, error) { calls++; return "", expected })
	if !errors.Is(err, expected) || calls != 1 {
		t.Fatal(calls, err)
	}
}

func TestContentBlocksKeepSectionAnchor(t *testing.T) {
	for _, test := range []struct {
		pattern *regexp.Regexp
		code    string
	}{
		{faqShortcode, "[faq id=1]"}, {formShortcode, "[form id=1]"}, {collectionShortcode, "[gallery id=1]"}, {moduleShortcode, `[module id=&#34;example&#34;]`},
	} {
		source := `<p class="align-center" id="section-contacts">` + test.code + `</p>`
		output, err := replaceContentBlocks(source, test.pattern, func(string) (string, error) { return "<section>Content</section>", nil })
		if err != nil || output != `<div id="section-contacts"><section>Content</section></div>` {
			t.Fatal(output, err)
		}
		output, err = replaceContentBlocks(source, test.pattern, func(code string) (string, error) { return code, nil })
		if err != nil || output != source {
			t.Fatal("duplicated unchanged anchor", output, err)
		}
	}
}
