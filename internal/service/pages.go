package service

import (
	"regexp"
	"strings"
)

var pageTemplate = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}\.html$`)

func ValidPageTemplate(name string) bool {
	if !pageTemplate.MatchString(name) {
		return false
	}
	switch name {
	case "layout.html", "home.html", "404.html", "seo.html", "search.html", "single.html", "archive.html", "header.html", "footer.html":
		return false
	}
	return true
}
func validContentPath(slug string) bool {
	for _, part := range strings.Split(slug, "/") {
		if !slugPattern.MatchString(part) || len(part) > 120 {
			return false
		}
	}
	return true
}
