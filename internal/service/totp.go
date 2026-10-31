package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
)

// RFC 6238's SHA-1 truncation with six digits and a 30-second step.
func totpCode(secret []byte, counter int64) string {
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 15)
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}
func verifyTOTP(secret, code string, now, last int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) != 20 {
		return 0, false
	}
	counter := now / 30
	for step := counter - 1; step <= counter+1; step++ {
		if step < 0 || step <= last {
			continue
		}
		if hmac.Equal([]byte(totpCode(key, step)), []byte(code)) {
			return step, true
		}
	}
	return 0, false
}
