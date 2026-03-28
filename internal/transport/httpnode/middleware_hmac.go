package httpnode

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	headerTimestamp = "X-Agent-Timestamp"
	headerSignature = "X-Agent-Signature"
)

func HMACAuthMiddleware(secret string, maxSkew time.Duration) func(next http.Handler) http.Handler {
	key := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tsRaw := r.Header.Get(headerTimestamp)
			sigHex := r.Header.Get(headerSignature)
			if tsRaw == "" || sigHex == "" {
				respondError(w, http.StatusUnauthorized, "missing auth headers")
				return
			}

			ts, err := strconv.ParseInt(tsRaw, 10, 64)
			if err != nil {
				respondError(w, http.StatusUnauthorized, "invalid timestamp")
				return
			}

			now := time.Now().UTC()
			reqTime := time.Unix(ts, 0).UTC()
			if reqTime.Before(now.Add(-maxSkew)) || reqTime.After(now.Add(maxSkew)) {
				respondError(w, http.StatusUnauthorized, "timestamp skew exceeded")
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				respondError(w, http.StatusBadRequest, "cannot read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			mac := hmac.New(sha256.New, key)
			_, _ = mac.Write([]byte(tsRaw))
			_, _ = mac.Write([]byte("."))
			_, _ = mac.Write(body)
			expected := mac.Sum(nil)

			provided, err := hex.DecodeString(sigHex)
			if err != nil || !hmac.Equal(expected, provided) {
				respondError(w, http.StatusUnauthorized, "invalid signature")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
