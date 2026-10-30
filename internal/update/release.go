// Package update verifies release metadata; container deployment belongs to Dokploy.
package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrRelease = errors.New("release metadata could not be verified")
var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Release struct {
	Format         int    `json:"format"`
	Version        string `json:"version"`
	MinimumVersion string `json:"minimum_version"`
	Schema         int    `json:"schema"`
	Image          string `json:"image"`
	Notes          string `json:"notes"`
}
type Signed struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

func versionParts(value string) ([3]int, bool) {
	var parts [3]int
	groups := stableVersion.FindStringSubmatch(value)
	if groups == nil {
		return parts, false
	}
	for i := range parts {
		parts[i], _ = strconv.Atoi(groups[i+1])
	}
	return parts, true
}
func Compare(a, b string) (int, error) {
	aa, ok := versionParts(a)
	if !ok {
		return 0, ErrRelease
	}
	bb, ok := versionParts(b)
	if !ok {
		return 0, ErrRelease
	}
	for i := range aa {
		if aa[i] < bb[i] {
			return -1, nil
		}
		if aa[i] > bb[i] {
			return 1, nil
		}
	}
	return 0, nil
}
func CurrentBase(value string) string {
	return strings.SplitN(strings.TrimPrefix(value, "v"), "-", 2)[0]
}
func (r Release) Validate(repository string) error {
	if r.Format != 1 || r.Schema < 1 || r.Schema > 100000 || len(r.Notes) > 12000 {
		return ErrRelease
	}
	if _, ok := versionParts(r.Version); !ok {
		return ErrRelease
	}
	if _, ok := versionParts(r.MinimumVersion); !ok {
		return ErrRelease
	}
	if c, err := Compare(r.MinimumVersion, r.Version); err != nil || c > 0 {
		return ErrRelease
	}
	prefix := repository + "@sha256:"
	if !strings.HasPrefix(r.Image, prefix) || !digest.MatchString(strings.TrimPrefix(r.Image, prefix)) {
		return ErrRelease
	}
	return nil
}
func (s Signed) Verify(publicKey, repository string) (Release, error) {
	var r Release
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize || len(s.Manifest) > 20000 || len(s.Signature) != ed25519.SignatureSize || !ed25519.Verify(key, s.Manifest, s.Signature) {
		return r, ErrRelease
	}
	decoder := json.NewDecoder(strings.NewReader(string(s.Manifest)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || r.Validate(repository) != nil {
		return Release{}, ErrRelease
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Release{}, ErrRelease
	}
	return r, nil
}
func HTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !HTTPS(r.URL.String()) {
			return ErrRelease
		}
		return nil
	}}
}
func read(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	if !HTTPS(address) {
		return nil, ErrRelease
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, ErrRelease
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, ErrRelease
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, ErrRelease
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, ErrRelease
	}
	return body, nil
}
func Fetch(ctx context.Context, client *http.Client, feed, key, repository string) (Signed, Release, error) {
	raw, err := read(ctx, client, feed, 20000)
	if err != nil {
		return Signed{}, Release{}, err
	}
	signature, err := read(ctx, client, feed+".sig", 128)
	if err != nil {
		return Signed{}, Release{}, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil {
		return Signed{}, Release{}, ErrRelease
	}
	signed := Signed{Manifest: raw, Signature: decoded}
	release, err := signed.Verify(key, repository)
	return signed, release, err
}
