package httpui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxiesNeverTrustClientHeadersByDefault(t *testing.T) {
	_, app := openApp(t, t.TempDir())
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "203.0.113.10:1234"
	request.Header.Set("X-Forwarded-For", "1.2.3.4")
	if app.Echo.IPExtractor == nil || app.Echo.IPExtractor(request) != "203.0.113.10" {
		t.Fatal("unsafe default IP extractor")
	}
	if err := app.ConfigureProxies(""); err != nil {
		t.Fatal(err)
	}
	if got := app.Echo.IPExtractor(request); got != "203.0.113.10" {
		t.Fatal("trusted client header", got)
	}
	if err := app.ConfigureProxies("10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if got := app.Echo.IPExtractor(request); got != "203.0.113.10" {
		t.Fatal("untrusted peer accepted", got)
	}
	request.RemoteAddr = "10.0.0.5:443"
	request.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.10")
	if got := app.Echo.IPExtractor(request); got != "203.0.113.10" {
		t.Fatal("wrong chain boundary", got)
	}
	if err := app.ConfigureProxies("not-a-cidr"); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
}

func TestLoginLimitsUseTrustedClientIP(t *testing.T) {
	_, app, _ := seoApp(t)
	if err := app.ConfigureProxies("10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	login := func(ip string) int {
		request := httptest.NewRequest("POST", "https://example.com/login", strings.NewReader("email=owner%40example.com&password=wrong-password"))
		request.RemoteAddr = "10.0.0.5:443"
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("X-Forwarded-For", ip)
		request.Header.Set("Cookie", "csrf="+token)
		request.Header.Set("X-CSRF-Token", token)
		response := httptest.NewRecorder()
		app.Echo.ServeHTTP(response, request)
		return response.Code
	}
	for i := 0; i < 5; i++ {
		if code := login("203.0.113.1"); code != 401 {
			t.Fatal(code)
		}
	}
	if code := login("203.0.113.1"); code != 429 {
		t.Fatal("client was not limited", code)
	}
	if code := login("203.0.113.2"); code != 401 {
		t.Fatal("another client was blocked", code)
	}
}
