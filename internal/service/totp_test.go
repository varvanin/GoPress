package service

import (
	"encoding/base32"
	"testing"
)

func TestTOTPMatchesRFC6238SHA1Vectors(t *testing.T) {
	// Six-digit values are the RFC's eight-digit SHA-1 results modulo one million.
	secret := []byte("12345678901234567890")
	for _, test := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		if got := totpCode(secret, test.at/30); got != test.code {
			t.Fatalf("at %d: %s != %s", test.at, got, test.code)
		}
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	counter, ok := verifyTOTP(encoded, "287082", 59, -1)
	if !ok || counter != 1 {
		t.Fatal(counter, ok)
	}
	if _, ok = verifyTOTP(encoded, "287082", 59, 1); ok {
		t.Fatal("replayed counter accepted")
	}
	if _, ok = verifyTOTP(encoded, "287082", 121, -1); ok {
		t.Fatal("expired code accepted")
	}
	for _, code := range []string{"28708", "a87082", "2870820"} {
		if _, ok = verifyTOTP(encoded, code, 59, -1); ok {
			t.Fatal("invalid code accepted")
		}
	}
}
