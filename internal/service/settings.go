package service

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"gopress/internal/store"
)

func (v *Service) SaveSettings(ctx context.Context, value store.Settings) error {
	value.Site.Title = strings.TrimSpace(value.Site.Title)
	value.Description = strings.TrimSpace(value.Description)
	value.Site.BaseURL = strings.TrimRight(strings.TrimSpace(value.Site.BaseURL), "/")
	u, err := url.Parse(value.Site.BaseURL)
	if value.FormRetentionDays < 1 || value.FormRetentionDays > 3650 || value.AuditRetentionDays < 1 || value.AuditRetentionDays > 3650 || value.Version < 0 || !utf8.ValidString(value.Site.Title+value.Description) || value.Site.Title == "" || utf8.RuneCountInString(value.Site.Title) > 200 || utf8.RuneCountInString(value.Description) > 500 || len(value.Site.BaseURL) > 2048 || err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalid
	}
	if _, err = time.LoadLocation(value.Site.Timezone); err != nil {
		return ErrInvalid
	}
	if value.Language != "ru" && value.Language != "en" && value.Language != "pl" {
		return ErrInvalid
	}
	if value.PostsPerPage < 1 || value.PostsPerPage > 100 || (value.HomeMode != "posts" && value.HomeMode != "page") || value.HomePageID < 0 || value.HomeMode == "page" && value.HomePageID < 1 {
		return ErrInvalid
	}
	if value.HomeMode == "posts" {
		value.HomePageID = 0
	}
	return v.Store.SaveSettings(ctx, value)
}
