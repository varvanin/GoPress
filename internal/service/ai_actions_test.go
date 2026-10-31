package service

import (
	"strings"
	"testing"
)

func TestAIActionsValidateBeforeRequestAndRejectPartialStructuredResults(t *testing.T) {
	for _, mode := range []string{"rewrite", "shorten", "continue", "seo"} {
		in, err := prepareAI(AIInput{Mode: mode, Language: "en", Text: "Reference"})
		if err != nil || in.Prompt == "" {
			t.Fatal(mode, in, err)
		}
	}
	for _, in := range []AIInput{{Mode: "agent", Text: "data"}, {Mode: "translate", Text: `{"wrong":true}`}, {Mode: "translate", Text: `[]`}, {Mode: "rewrite"}} {
		if _, err := prepareAI(in); err == nil {
			t.Fatal("invalid action accepted", in)
		}
	}
	in := AIInput{Mode: "translate", Text: `["First","Second"]`}
	for _, raw := range []string{`{"title":"English","texts":["One"]}`, `{"title":"English","texts":["One",""]}`, `{"title":"English","texts":["One","Two"],"script":"bad"}`, `{"title":"English","texts":["One","Two"]} trailing`} {
		if _, err := validateAIResult(in, raw); err == nil {
			t.Fatal("partial or malformed result accepted", raw)
		}
	}
	if _, err := validateAIResult(in, `{"title":"English","texts":["<script>literal</script>","Two"]}`); err != nil {
		t.Fatal("plain text rejected", err)
	}
	in.Mode = "seo"
	if _, err := validateAIResult(in, `{"title":"Title","description":"Description"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := validateAIResult(in, `{"title":"`+strings.Repeat("a", 201)+`","description":"Description"}`); err == nil {
		t.Fatal("oversized SEO accepted")
	}
}
