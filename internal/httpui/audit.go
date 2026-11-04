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
		"user.2fa_enabled": "Enable 2FA", "user.2fa_disabled": "Disable 2FA", "user.2fa_reset": "Offline 2FA reset", "user.recovery_codes_rotated": "Recovery codes replaced", "user.session_revoked": "Session ended", "user.sessions_revoked": "Other sessions ended",
		"content.schedule_failed":        "Scheduled publication failed: check the parent and content URL",
		"POST /admin/themes/editor/save": "Template file edited", "POST /admin/themes/editor/copy": "Template copy created",
		"POST /admin/modules/upload": "Module installation", "POST /admin/modules/package/save": "Module settings saved", "POST /admin/modules/package/toggle": "Module enabled or disabled", "POST /admin/modules/package/delete": "Module deletion",
		"POST /admin/modules/faq/toggle": "FAQ module settings", "POST /admin/modules/faq/save": "FAQ group saved.", "POST /admin/modules/faq/delete": "Delete FAQ group",
		"user.login": "Admin sign-in", "user.login_failed": "Failed sign-in", "user.save": "User changes", "user.password": "Password change", "user.reset": "Password recovery", "cli.reset_password": "Password recovery via CLI", "user.reset_requested": "Recovery link sent", "user.reset_delivery_failed": "Recovery email failed",
		"content.move": "Page addresses changed", "content.scheduled": "Scheduled publication", "content.create": "Content creation", "content.update": "Content saved", "content.publish": "Content publication", "content.trash": "Moved to trash", "content.restore": "Content restoration", "content.delete": "Permanent content deletion", "content.unpublish": "Content unpublished", "content.cancel_schedule": "Scheduled publication cancelled", "forms.delete_entries": "Submissions deleted", "forms.export": "Submissions exported",
		"POST /admin/tools/backups/create": "Backup creation", "POST /admin/tools/backups/restore": "Site restoration", "POST /admin/tools/backups/verify": "Backup verification", "POST /admin/tools/backups/delete": "Backup deletion", "POST /admin/tools/backups/import": "Backup import", "POST /admin/tools/maintenance": "Maintenance mode changes", "POST /admin/tools/retention": "Audit retention changes", "POST /admin/themes": "Template activation", "POST /admin/appearance": "Template activation", "POST /admin/themes/upload": "Template installation", "POST /admin/themes/delete": "Template deletion", "POST /admin/pages/:id/schedule": "Publication scheduled", "POST /admin/forms/save": "Form saved", "POST /admin/forms/:form/delete": "Form deletion", "POST /admin/logout": "Admin sign-out",
	}
	if label, ok := labels[action]; ok {
		return i18n.T(ctx, label)
	}
	for _, item := range []struct{ prefix, label string }{{"POST /admin/settings/", "Site settings changes"}, {"POST /admin/media", "Media library changes"}, {"POST /admin/menus", "Menu saved"}, {"POST /admin/posts/terms", "Category and tag changes"}, {"POST /admin/seo", "SEO changes"}, {"POST /admin/forms/", "Form changes"}} {
		if strings.HasPrefix(action, item.prefix) {
			return i18n.T(ctx, item.label)
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
