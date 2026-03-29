package httpnode

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpcommon"
	"log/slog"
)

type Options struct {
	Logger           *slog.Logger
	ReadinessTimeout time.Duration
	Runtime          runtime.VPNRuntime
	HMACSecret       string
	HMACMaxSkew      time.Duration
}

func NewRouter(opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(httpcommon.AccessLogMiddleware(opts.Logger))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), opts.ReadinessTimeout)
		defer cancel()

		if err := opts.Runtime.Health(ctx); err != nil {
			respondJSON(w, http.StatusOK, map[string]any{
				"status":  "degraded",
				"service": "node-agent",
				"runtime": "unavailable",
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "node-agent",
			"runtime": "available",
		})
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), opts.ReadinessTimeout)
		defer cancel()

		if err := opts.Runtime.Health(ctx); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":  "not_ready",
				"reason":  "runtime",
				"runtime": "unavailable",
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{"status": "ready", "service": "node-agent", "runtime": "available"})
	})

	r.Group(func(r chi.Router) {
		r.Use(HMACAuthMiddleware(opts.HMACSecret, opts.HMACMaxSkew))
		r.Post("/agent/v1/operations/apply-peer", applyPeerHandler(opts.Logger, opts.Runtime))
		r.Post("/agent/v1/operations/revoke-peer", revokePeerHandler(opts.Logger, opts.Runtime))
		r.Get("/agent/v1/traffic/counters", trafficCountersHandler(opts.Logger, opts.Runtime))
	})

	return r
}

func respondJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
