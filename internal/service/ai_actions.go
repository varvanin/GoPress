package service

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type AITranslation struct {
	Title string   `json:"title"`
	Texts []string `json:"texts"`
}
type AISEO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func prepareAI(in AIInput) (AIInput, error) {
	instruction := ""
	switch in.Mode {
	case "", "generate":
		return in, nil
	case "rewrite":
		instruction = "Rewrite the reference text clearly, preserving its meaning. Return only replacement text."
	case "shorten":
		instruction = "Shorten the reference text without losing essential meaning. Return only replacement text."
	case "continue":
		instruction = "Continue the reference text naturally. Return only the new continuation, without repeating the reference."
	case "seo":
		instruction = "Suggest a factual SEO title and description for the reference. Return only JSON with string fields title (at most 200 characters) and description (at most 500 characters)."
	case "translate":
		var texts []string
		if decodeStrict(in.Text, &texts) != nil || len(texts) == 0 || len(texts) > 10000 {
			return in, ErrInvalid
		}
		instruction = "Translate the title and each string in the reference JSON array into language code " + in.Language + ". Preserve the array length and order. Return only JSON with string title (at most 200 characters) and string array texts. Do not add facts or explanations."
	default:
		return in, ErrInvalid
	}
	if in.Text == "" {
		return in, ErrInvalid
	}
	if in.Prompt != "" {
		instruction += "\nAdditional request: " + in.Prompt
	}
	in.Prompt = instruction
	return in, nil
}
func validateAIResult(in AIInput, text string) (string, error) {
	switch in.Mode {
	case "translate":
		var source []string
		var result AITranslation
		if json.Unmarshal([]byte(in.Text), &source) != nil || decodeStrict(text, &result) != nil || strings.TrimSpace(result.Title) == "" || utf8.RuneCountInString(result.Title) > 200 || len(result.Texts) != len(source) {
			return "", ErrAIUnavailable
		}
		for i, value := range result.Texts {
			if !utf8.ValidString(value) || (strings.TrimSpace(source[i]) != "" && strings.TrimSpace(value) == "") {
				return "", ErrAIUnavailable
			}
		}
		raw, err := json.Marshal(result)
		if err != nil || len(raw) > 64000 {
			return "", ErrAIUnavailable
		}
		return string(raw), nil
	case "seo":
		var result AISEO
		if decodeStrict(text, &result) != nil || strings.TrimSpace(result.Title) == "" || strings.TrimSpace(result.Description) == "" || utf8.RuneCountInString(result.Title) > 200 || utf8.RuneCountInString(result.Description) > 500 {
			return "", ErrAIUnavailable
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return "", ErrAIUnavailable
		}
		return string(raw), nil
	}
	return text, nil
}
