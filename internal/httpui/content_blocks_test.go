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
