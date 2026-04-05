package httpnode

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/auth"
)

func HMACAuthMiddleware(secret string, maxSkew time.Duration) func(next http.Handler) http.Handler {
	key := []byte(secret)
	logger := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tsRaw := r.Header.Get(auth.HeaderTimestamp)
			sigHex := r.Header.Get(auth.HeaderSignature)
			hasTS := tsRaw != ""
			hasSig := sigHex != ""
			if tsRaw == "" || sigHex == "" {
				logger.Debug("agent signature verification",
					slog.String("path", r.URL.Path),
					slog.Bool("has_auth_header", hasSig),
					slog.Bool("has_timestamp_header", hasTS),
					slog.String("validation_result", "rejected"),
					slog.String("reason", "missing_auth_headers"),
				)
				respondError(w, http.StatusUnauthorized, "missing auth headers")
				return
			}

			ts, err := strconv.ParseInt(tsRaw, 10, 64)
			if err != nil {
				logger.Debug("agent signature verification",
					slog.String("path", r.URL.Path),
					slog.Bool("has_auth_header", hasSig),
					slog.Bool("has_timestamp_header", hasTS),
					slog.String("validation_result", "rejected"),
					slog.String("reason", "invalid_timestamp"),
				)
				respondError(w, http.StatusUnauthorized, "invalid timestamp")
				return
			}

			now := time.Now().UTC()
			reqTime := time.Unix(ts, 0).UTC()
			if reqTime.Before(now.Add(-maxSkew)) || reqTime.After(now.Add(maxSkew)) {
				logger.Debug("agent signature verification",
					slog.String("path", r.URL.Path),
					slog.Bool("has_auth_header", hasSig),
					slog.Bool("has_timestamp_header", hasTS),
					slog.String("validation_result", "rejected"),
					slog.String("reason", "timestamp_skew_exceeded"),
				)
				respondError(w, http.StatusUnauthorized, "timestamp skew exceeded")
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				respondError(w, http.StatusBadRequest, "cannot read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			if !auth.Verify(key, tsRaw, body, sigHex) {
				logger.Debug("agent signature verification",
					slog.String("path", r.URL.Path),
					slog.Bool("has_auth_header", hasSig),
					slog.Bool("has_timestamp_header", hasTS),
					slog.String("validation_result", "rejected"),
					slog.String("reason", "signature_mismatch"),
				)
				respondError(w, http.StatusUnauthorized, "invalid signature")
				return
			}
			logger.Debug("agent signature verification",
				slog.String("path", r.URL.Path),
				slog.Bool("has_auth_header", hasSig),
				slog.Bool("has_timestamp_header", hasTS),
				slog.String("validation_result", "accepted"),
			)

			next.ServeHTTP(w, r)
		})
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
