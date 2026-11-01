package httpui

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

func auditDate(ctx context.Context, at int64, zone string) string {
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.UTC
	}
	return time.Unix(at, 0).In(location).Format("02.01.2006 15:04:05 MST")
}
func auditActor(ctx context.Context, e store.AuditEvent) string {
	if e.Email != "" {
		return e.Email
	}
	if e.UserID > 0 {
		return i18n.T(ctx, "User") + " #" + strconv.FormatInt(e.UserID, 10)
	}
	if e.Action == "user.login_failed" {
		return i18n.T(ctx, "Not signed in")
	}
	return i18n.T(ctx, "System")
}
func auditAction(ctx context.Context, action string) string {
	labels := map[string]string{
		"user.2fa_enabled": "Enable 2FA", "user.2fa_disabled": "Disable 2FA", "user.2fa_reset": "Restore access without 2FA", "user.recovery_codes_rotated": "Replace recovery codes", "user.session_revoked": "Session terminated", "user.sessions_revoked": "Other sessions terminated",
		"content.schedule_failed":        "Scheduled publication failed: check the parent and URL",
		"POST /admin/themes/editor/save": "Template file edited", "POST /admin/themes/editor/copy": "Template copy created",
		"POST /admin/modules/upload": "Module installed", "POST /admin/modules/package/save": "Module settings saved", "POST /admin/modules/package/toggle": "Module toggled", "POST /admin/modules/package/delete": "Module deleted",
		"POST /admin/modules/faq/toggle": "FAQ module configured", "POST /admin/modules/faq/save": "FAQ group saved", "POST /admin/modules/faq/delete": "FAQ group deleted",
		"user.login": "Admin login", "user.login_failed": "Failed login", "user.save": "User updated", "user.password": "Password changed", "user.reset": "Password recovered", "cli.reset_password": "Password recovered via CLI", "user.reset_requested": "Recovery link sent", "user.reset_delivery_failed": "Recovery link delivery failed",
		"content.move": "Page addresses changed", "content.scheduled": "Scheduled publication", "content.create": "Content created", "content.update": "Content saved", "content.publish": "Content published", "content.trash": "Moved to trash", "content.restore": "Content restored", "content.delete": "Content permanently deleted", "content.unpublish": "Content unpublished", "content.cancel_schedule": "Schedule cancelled", "forms.delete_entries": "Submissions deleted", "forms.export": "Submissions exported",
		"POST /admin/tools/backups/create": "Backup created", "POST /admin/tools/backups/restore": "Site restored", "POST /admin/tools/backups/verify": "Backup verified", "POST /admin/tools/backups/delete": "Backup deleted", "POST /admin/tools/backups/import": "Backup imported", "POST /admin/tools/maintenance": "Maintenance mode changed", "POST /admin/tools/retention": "Log retention period changed", "POST /admin/themes": "Template activated", "POST /admin/appearance": "Template activated", "POST /admin/themes/upload": "Template installed", "POST /admin/themes/delete": "Template deleted", "POST /admin/pages/:id/schedule": "Publication scheduled", "POST /admin/forms/save": "Form saved", "POST /admin/forms/:form/delete": "Form deleted", "POST /admin/logout": "Admin logout",
	}
	if label, ok := labels[action]; ok {
		return label
	}
	for _, item := range []struct{ prefix, label string }{{"POST /admin/settings/", "Site settings changed"}, {"POST /admin/media", "Media library changed"}, {"POST /admin/menus", "Menu saved"}, {"POST /admin/posts/terms", "Categories and tags changed"}, {"POST /admin/seo", "SEO changed"}, {"POST /admin/forms/", "Form changed"}} {
		if strings.HasPrefix(action, item.prefix) {
			return item.label
		}
	}
	return i18n.T(ctx, "Admin action")
}
func auditObject(ctx context.Context, e store.AuditEvent) string {
	var target struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	if json.Unmarshal([]byte(e.Object), &target) == nil && target.Title != "" {
		return target.Title + " · #" + strconv.FormatInt(target.ID, 10)
	}
	if id, err := strconv.ParseInt(e.Object, 10, 64); err == nil && id > 0 {
		label := "Content"
		if strings.HasPrefix(e.Action, "user.") || e.Action == "cli.reset_password" {
			label = "User"
		} else if strings.Contains(e.Action, "forms") {
			label = "Form"
		}
		return i18n.T(ctx, label) + " #" + e.Object
	}
	if e.Object != "" {
		return e.Object
	}
	if strings.Contains(e.Action, "/settings") || strings.Contains(e.Action, "/tools/") || strings.Contains(e.Action, "/themes") {
		return i18n.T(ctx, "Site")
	}
	return "—"
}
