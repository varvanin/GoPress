package httpui

import (
	"strings"
	"testing"
)

func TestAdminFeedbackSurvivesNavigation(t *testing.T) {
	_, _, b := seoApp(t)
	full := b.request("GET", "/admin", "", "", false)
	status(t, full, 200)
	for _, marker := range []string{`id="system-toasts"`, `role="alertdialog"`, `data-confirm-result="cancel"`, `data-confirm-result="accept"`, `data-templ-disable-pointer-dismissal="true"`} {
		if strings.Count(full.Body.String(), marker) != 1 {
			t.Errorf("feedback shell must contain one %s", marker)
		}
	}
	partial := adminHTML(b, "GET", "/admin/themes", nil, true, false, false)
	status(t, partial, 200)
	if strings.Contains(partial.Body.String(), `id="system-toasts"`) || strings.Contains(partial.Body.String(), `id="system-confirm"`) {
		t.Fatal("SPA partial duplicated persistent feedback shell")
	}
}
