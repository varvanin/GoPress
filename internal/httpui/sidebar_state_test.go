package httpui

import (
	"net/http"
	"strings"
	"testing"
)

func TestSidebarStateRestoredByFullAndHistoryRequests(t *testing.T) {
	_, _, b := seoApp(t)
	for _, tc := range []struct{ cookie, state, mode string }{{"", "expanded", ""}, {"false", "collapsed", "icon"}, {"true", "expanded", ""}, {"unexpected", "expanded", ""}} {
		if tc.cookie == "" {
			delete(b.cookies, "sidebar_state")
		} else {
			b.cookies["sidebar_state"] = &http.Cookie{Name: "sidebar_state", Value: tc.cookie}
		}
		for _, history := range []bool{false, true} {
			response := adminHTML(b, "GET", "/admin/tools?tab=system", nil, history, history, false)
			status(t, response, 200)
			body := response.Body.String()
			if !strings.Contains(body, `data-state="`+tc.state+`"`) || !strings.Contains(body, `data-collapsible="`+tc.mode+`"`) {
				t.Fatalf("sidebar cookie=%q history=%v was not restored", tc.cookie, history)
			}
		}
	}
	b.cookies["sidebar_state"] = &http.Cookie{Name: "sidebar_state", Value: "false"}
	fragment := adminHTML(b, "GET", "/admin/seo", nil, true, false, false)
	status(t, fragment, 200)
	if strings.Contains(fragment.Body.String(), `data-templ-sidebar-id="gopress-sidebar"`) {
		t.Fatal("partial navigation replaces sidebar")
	}
}
