package httpui

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopress/internal/service"
	"gopress/internal/store"
	"gopress/internal/update"
	"gopress/internal/version"
)

func TestUpdatesRequireAdministratorAndEscapeReleaseNotes(t *testing.T) {
	s, _, b := seoApp(t)
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("GOPRESS_UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(public))
	t.Setenv("GOPRESS_DOKPLOY_API_KEY", "PRIVATE-DOKPLOY-KEY")
	release := update.Release{Format: 1, Version: "9.0.0", MinimumVersion: "0.2.0", Schema: store.SchemaVersion, Image: version.ImageRepository + "@sha256:" + strings.Repeat("a", 64), Notes: "<script>not executable</script>"}
	raw, _ := json.Marshal(release)
	signed, _ := json.Marshal(update.Signed{Manifest: raw, Signature: ed25519.Sign(private, raw)})
	root := filepath.Join(s.Dir, "updates")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release.json"), signed, 0600); err != nil {
		t.Fatal(err)
	}
	response := b.request("GET", "/admin/updates", "", "", false)
	status(t, response, 200)
	body := response.Body.String()
	if strings.Contains(body, "PRIVATE-DOKPLOY-KEY") || strings.Contains(body, "<script>not executable") || !strings.Contains(body, "&lt;script&gt;") || strings.Contains(body, `action="/admin/updates/install"`) {
		t.Fatal("exposed secret/HTML or offered install in native mode")
	}
	status(t, b.request("POST", "/admin/updates/check", "", "application/x-www-form-urlencoded", false), 403)
	status(t, b.request("POST", "/admin/updates/install", "version=9.0.0", "application/x-www-form-urlencoded", true), 400)
	if err := os.WriteFile(filepath.Join(s.Dir, "update-in-progress"), []byte("9.0.0"), 0600); err != nil {
		t.Fatal(err)
	}
	status(t, b.request("POST", "/admin/settings/language", "language=en", "application/x-www-form-urlencoded", true), 503)
	status(t, b.request("POST", "/admin/tools/maintenance", "enabled=false", "application/x-www-form-urlencoded", true), 503)
	state := service.UpdateState{Phase: "requested", Started: time.Now().Unix()}
	raw, _ = json.Marshal(state)
	if err := os.WriteFile(filepath.Join(root, "state.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	response = b.request("GET", "/admin/updates/status", "", "", false)
	status(t, response, 200)
	if !strings.Contains(response.Body.String(), "every 5s") {
		t.Fatal("pending status does not refresh")
	}
	id, err := s.SaveUser(context.Background(), store.User{Email: "author@example.com", Role: "author"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	token := service.Token()
	if err = s.LoginSession(service.Hash(token), id, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	b.cookies["session"] = &http.Cookie{Name: "session", Value: token}
	status(t, b.request("GET", "/admin/updates", "", "", false), 403)
	status(t, b.request("GET", "/admin/updates/status", "", "", false), 403)
}

func TestReadinessCanRequireAnInstalledSite(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		app.Echo.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	status(t, get("/readyz"), 200)
	status(t, get("/readyz?installed=1"), 503)
	if err := s.Install(context.Background(), store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	status(t, get("/readyz?installed=1"), 200)
}
