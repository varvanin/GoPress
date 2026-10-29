package httpui

import (
	"fmt"
	"regexp"
)

// Bound expansion before the outer template receives the assembled document.
func replaceContentBlocks(body string, pattern *regexp.Regexp, render func(string) (string, error)) (string, error) {
	if len(body) > 8<<20 {
		return "", fmt.Errorf("content output exceeds 8 MiB")
	}
	matches := pattern.FindAllStringIndex(body, 51)
	if len(matches) == 0 {
		return body, nil
	}
	if len(matches) > 50 {
		return "", fmt.Errorf("too many content blocks")
	}
	size := len(body)
	var renderErr error
	result := pattern.ReplaceAllStringFunc(body, func(code string) string {
		if renderErr != nil {
			return ""
		}
		value, err := render(code)
		if err != nil {
			renderErr = err
			return ""
		}
		size += len(value) - len(code)
		if size > 8<<20 {
			renderErr = fmt.Errorf("content output exceeds 8 MiB")
			return ""
		}
		return value
	})
	if renderErr != nil {
		return "", renderErr
	}
	return result, nil
}
