package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

func HostSignatureMessage(method, path, timestamp string, body []byte) []byte {
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%x", method, path, timestamp, sha256.Sum256(body)))
}
func CanonicalHostInteger(s string) (int64, bool) {
	n, e := strconv.ParseInt(s, 10, 64)
	return n, e == nil && n > 0 && strconv.FormatInt(n, 10) == s
}
func VerifyHostSignature(key ed25519.PublicKey, method, path, timestamp, signature string, body []byte, now time.Time) bool {
	ts, ok := CanonicalHostInteger(timestamp)
	if !ok {
		return false
	}
	sec := now.Unix()
	if ts > sec {
		if uint64(ts)-uint64(sec) > 300 {
			return false
		}
	} else if uint64(sec)-uint64(ts) > 300 {
		return false
	}
	sig, e := base64.StdEncoding.DecodeString(signature)
	if e != nil || len(sig) != 64 || base64.StdEncoding.EncodeToString(sig) != signature || len(key) != 32 {
		return false
	}
	return ed25519.Verify(key, HostSignatureMessage(method, path, timestamp, body), sig)
}
