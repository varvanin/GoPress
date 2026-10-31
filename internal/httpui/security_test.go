package httpui

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testAuthenticatorCode(secret string) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
}
func TestTwoFactorCredentialsDownloadWithoutHTMLOrTokenExposure(t *testing.T) {
	s, _, b := seoApp(t)
	password := "correct horse battery staple"
	page := b.request("GET", "/admin/account/security", "", "", false)
	status(t, page, 200)
	if strings.Contains(page.Body.String(), b.cookies["session"].Value) {
		t.Fatal("session bearer token exposed in HTML")
	}
	setup := adminHTML(b, "POST", "/admin/account/2fa/setup", url.Values{"password": {password}}, false, false, false)
	status(t, setup, 403)
	setup = adminHTML(b, "POST", "/admin/account/2fa/setup", url.Values{"password": {password}}, false, false, true)
	status(t, setup, 200)
	if setup.Header().Get("Content-Type") != "text/plain; charset=utf-8" || !strings.Contains(setup.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("setup secret was not a private download")
	}
	parts := strings.Split(setup.Body.String(), "\n\n")
	if len(parts) < 3 {
		t.Fatal("invalid setup download")
	}
	secret := parts[1]
	page = b.request("GET", "/admin/account/security", "", "", false)
	status(t, page, 200)
	if strings.Contains(page.Body.String(), secret) {
		t.Fatal("setup key appeared in HTML")
	}
	enabled := adminHTML(b, "POST", "/admin/account/2fa/enable", url.Values{"password": {password}, "code": {testAuthenticatorCode(secret)}}, false, false, true)
	status(t, enabled, 200)
	if !strings.Contains(enabled.Header().Get("Content-Disposition"), "gopress-recovery-codes.txt") {
		t.Fatal("missing recovery download")
	}
	status(t, b.request("GET", "/admin/account/security", "", "", false), 401)
	profile, err := s.SecurityProfile(1)
	if err != nil || !profile.Enabled {
		t.Fatal(profile, err)
	}
}
