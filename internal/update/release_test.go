package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func signedFixture(t *testing.T) (Signed, string, Release) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	release := Release{Format: 1, Version: "1.10.0", MinimumVersion: "0.2.0", Schema: 16, Image: "ghcr.io/varvanin/gopress@sha256:" + strings.Repeat("a", 64), Notes: "Fixes <script>plain notes</script>"}
	raw, _ := json.Marshal(release)
	return Signed{Manifest: raw, Signature: ed25519.Sign(private, raw)}, base64.StdEncoding.EncodeToString(public), release
}
func TestSignedReleaseRejectsTamperingAndUntrustedImages(t *testing.T) {
	signed, key, release := signedFixture(t)
	if got, err := signed.Verify(key, "ghcr.io/varvanin/gopress"); err != nil || got != release {
		t.Fatal(got, err)
	}
	for _, key := range []string{"", strings.Repeat("a", 44)} {
		if _, err := signed.Verify(key, "ghcr.io/varvanin/gopress"); err == nil {
			t.Fatal("accepted wrong key")
		}
	}
	changed := signed
	changed.Manifest = append([]byte(nil), signed.Manifest...)
	changed.Manifest[4] ^= 1
	if _, err := changed.Verify(key, "ghcr.io/varvanin/gopress"); err == nil {
		t.Fatal("accepted modified payload")
	}
	if _, err := signed.Verify(key, "ghcr.io/another/project"); err == nil {
		t.Fatal("accepted another repository")
	}
	for _, image := range []string{"ghcr.io/varvanin/gopress:latest", "ghcr.io/varvanin/gopress@sha256:short", "https://evil.test/image"} {
		release.Image = image
		if release.Validate("ghcr.io/varvanin/gopress") == nil {
			t.Fatal(image)
		}
	}
}
func TestReleaseComparisonAndTransportBounds(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"1.10.0", "1.9.0", 1}, {"1.0.0", "1.0.0", 0}, {"0.9.0", "1.0.0", -1}} {
		got, err := Compare(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	for _, value := range []string{"latest", "01.2.3", "1.0.0-rc1", "1.2", "999999999999.0.0"} {
		if _, err := Compare(value, "1.0.0"); err == nil {
			t.Fatal(value)
		}
	}
	signed, key, _ := signedFixture(t)
	client := Client(time.Second)
	client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		body := string(signed.Manifest)
		if strings.HasSuffix(r.URL.Path, ".sig") {
			body = base64.StdEncoding.EncodeToString(signed.Signature)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if _, _, err := Fetch(context.Background(), client, "https://release.test/update.json", key, "ghcr.io/varvanin/gopress"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Fetch(context.Background(), client, "http://release.test/update.json", key, "ghcr.io/varvanin/gopress"); err == nil {
		t.Fatal("accepted HTTP feed")
	}
	client.Transport = transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 20001)))}, nil
	})
	if _, _, err := Fetch(context.Background(), client, "https://release.test/update.json", key, "ghcr.io/varvanin/gopress"); err == nil {
		t.Fatal("accepted oversized manifest")
	}
}
