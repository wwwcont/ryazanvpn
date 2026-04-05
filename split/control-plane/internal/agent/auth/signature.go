package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const (
	HeaderTimestamp = "X-Agent-Timestamp"
	HeaderSignature = "X-Agent-Signature"
)

func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret []byte, timestamp string, body []byte, providedHex string) bool {
	provided, err := hex.DecodeString(providedHex)
	if err != nil {
		return false
	}
	expected := hmac.New(sha256.New, secret)
	_, _ = expected.Write([]byte(timestamp))
	_, _ = expected.Write([]byte("."))
	_, _ = expected.Write(body)
	return hmac.Equal(expected.Sum(nil), provided)
}
