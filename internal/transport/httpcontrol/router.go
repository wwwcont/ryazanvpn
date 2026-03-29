package httpcontrol

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpcommon"
	"log/slog"
)

type NodeAdminReader interface {
	ListAll(ctx context.Context) ([]*node.Node, error)
}

type UserAdminReader interface {
	List(ctx context.Context) ([]*user.User, error)
}

type DeviceAdminReader interface {
	List(ctx context.Context) ([]*device.Device, error)
}

type InviteCodeAdminRepo interface {
	Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error)
	RevokeByID(ctx context.Context, id string) (*invitecode.InviteCode, error)
}

type AuditLogger interface {
	Create(ctx context.Context, in audit.CreateParams) (*audit.Log, error)
}

type Options struct {
	Logger            *slog.Logger
	PG                *pgxpool.Pool
	RedisClient       *redis.Client
	ReadinessTimeout  time.Duration
	DownloadUC        *app.DownloadDeviceConfigByToken
	AdminSecret       string
	AdminSecretHeader string
	Nodes             NodeAdminReader
	Users             UserAdminReader
	Devices           DeviceAdminReader
	InviteCodes       InviteCodeAdminRepo
	AuditLogs         AuditLogger
	TelegramWebhook   http.Handler
}

var inviteCodePattern = regexp.MustCompile(`^\d{4}$`)

func NewRouter(opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(httpcommon.AccessLogMiddleware(opts.Logger))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "control-plane",
		})
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), opts.ReadinessTimeout)
		defer cancel()

		if err := opts.PG.Ping(ctx); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "postgres"})
			return
		}

		if err := opts.RedisClient.Ping(ctx).Err(); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "redis"})
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{"status": "ready", "service": "control-plane"})
	})

	r.Get("/api/v1/metrics/overview", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var totalTraffic int64
		if err := opts.PG.QueryRow(ctx, `SELECT COALESCE(SUM(total_bytes),0) FROM traffic_usage_daily`).Scan(&totalTraffic); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load total traffic"})
			return
		}
		var traffic30 int64
		if err := opts.PG.QueryRow(ctx, `SELECT COALESCE(SUM(total_bytes),0) FROM traffic_usage_daily WHERE usage_date >= CURRENT_DATE - INTERVAL '29 day'`).Scan(&traffic30); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load 30d traffic"})
			return
		}
		var activeUsers int
		if err := opts.PG.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM access_grants WHERE status='active' AND expires_at > NOW()`).Scan(&activeUsers); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load active users"})
			return
		}
		var handshakeCount int
		_ = opts.PG.QueryRow(ctx, `SELECT COUNT(*) FROM device_accesses WHERE status = 'active'`).Scan(&handshakeCount)
		respondJSON(w, http.StatusOK, map[string]any{
			"total_traffic_bytes":        totalTraffic,
			"traffic_last_30_days_bytes": traffic30,
			"active_users":               activeUsers,
			"latest_handshake_count":     handshakeCount,
			"avg_latency_ms":             nil,
			"avg_speed_mbps":             nil,
			"latency_status":             "not_available",
			"speed_status":               "not_available",
		})
	})

	r.Get("/api/v1/metrics/nodes", func(w http.ResponseWriter, r *http.Request) {
		rows, err := opts.PG.Query(r.Context(), `SELECT id::text, name, status, current_load, user_capacity, agent_base_url, vpn_endpoint, last_seen_at FROM vpn_nodes ORDER BY name`)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load nodes"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, name, status, agentURL, endpoint string
			var load, capacity int
			var lastSeen *time.Time
			if err := rows.Scan(&id, &name, &status, &load, &capacity, &agentURL, &endpoint, &lastSeen); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan node"})
				return
			}
			nodeLoad := 0.0
			if capacity > 0 {
				nodeLoad = float64(load) / float64(capacity)
			}
			items = append(items, map[string]any{
				"id":             id,
				"name":           name,
				"status":         status,
				"current_load":   load,
				"user_capacity":  capacity,
				"node_load":      nodeLoad,
				"agent_base_url": agentURL,
				"vpn_endpoint":   endpoint,
				"last_seen_at":   lastSeen,
			})
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/api/v1/metrics/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimSpace(chi.URLParam(r, "id"))
		if uid == "" {
			respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		var total int64
		if err := opts.PG.QueryRow(r.Context(), `SELECT COALESCE(SUM(tud.total_bytes),0) FROM traffic_usage_daily tud JOIN devices d ON d.id=tud.device_id WHERE d.user_id = $1`, uid).Scan(&total); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load user total traffic"})
			return
		}
		var last30 int64
		if err := opts.PG.QueryRow(r.Context(), `SELECT COALESCE(SUM(tud.total_bytes),0) FROM traffic_usage_daily tud JOIN devices d ON d.id=tud.device_id WHERE d.user_id = $1 AND tud.usage_date >= CURRENT_DATE - INTERVAL '29 day'`, uid).Scan(&last30); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load user 30d traffic"})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"user_id":                    uid,
			"total_traffic_bytes":        total,
			"traffic_last_30_days_bytes": last30,
			"avg_latency_ms":             nil,
			"avg_speed_mbps":             nil,
			"latency_status":             "not_available",
			"speed_status":               "not_available",
		})
	})

	r.Get("/download/config/{token}", func(w http.ResponseWriter, r *http.Request) {
		if opts.DownloadUC == nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "download service disabled"})
			return
		}

		token := chi.URLParam(r, "token")
		if token == "" {
			respondJSON(w, http.StatusBadRequest, map[string]any{"error": "token is required"})
			return
		}

		cfg, err := opts.DownloadUC.Execute(r.Context(), token)
		if err != nil {
			respondJSON(w, http.StatusForbidden, map[string]any{"error": "invalid or expired token"})
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `attachment; filename="amneziawg.conf"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cfg))
	})

	r.Post("/internal/telegram/webhook", func(w http.ResponseWriter, r *http.Request) {
		if opts.TelegramWebhook == nil {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "telegram webhook is not configured"})
			return
		}
		opts.TelegramWebhook.ServeHTTP(w, r)
	})

	r.Route("/admin", func(ar chi.Router) {
		ar.Use(adminAuthMiddleware(opts.AdminSecretHeader, opts.AdminSecret))

		ar.Get("/nodes", func(w http.ResponseWriter, r *http.Request) {
			if opts.Nodes == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "nodes repository not configured"})
				return
			}
			items, err := opts.Nodes.ListAll(r.Context())
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list nodes"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		ar.Get("/users", func(w http.ResponseWriter, r *http.Request) {
			if opts.Users == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "users repository not configured"})
				return
			}
			items, err := opts.Users.List(r.Context())
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list users"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		ar.Get("/devices", func(w http.ResponseWriter, r *http.Request) {
			if opts.Devices == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "devices repository not configured"})
				return
			}
			items, err := opts.Devices.List(r.Context())
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list devices"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		ar.Post("/invite-codes", func(w http.ResponseWriter, r *http.Request) {
			if opts.InviteCodes == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "invite code repository not configured"})
				return
			}

			var req struct {
				Code            string  `json:"code"`
				MaxActivations  *int    `json:"max_activations"`
				ExpiresAt       *string `json:"expires_at"`
				CreatedByUserID *string `json:"created_by_user_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
				return
			}

			code := strings.TrimSpace(req.Code)
			if code == "" {
				code = generateInviteCode()
			} else if !inviteCodePattern.MatchString(code) {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "code must be exactly 4 digits"})
				return
			}

			maxActivations := 1
			if req.MaxActivations != nil {
				maxActivations = *req.MaxActivations
			}
			if maxActivations <= 0 {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "max_activations must be > 0"})
				return
			}

			var expiresAt *time.Time
			if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
				ts, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt))
				if err != nil {
					respondJSON(w, http.StatusBadRequest, map[string]any{"error": "expires_at must be RFC3339"})
					return
				}
				ts = ts.UTC()
				expiresAt = &ts
			}

			created, err := opts.InviteCodes.Create(r.Context(), invitecode.CreateParams{
				Code:            code,
				Status:          invitecode.CodeStatusActive,
				MaxActivations:  maxActivations,
				ExpiresAt:       expiresAt,
				CreatedByUserID: req.CreatedByUserID,
			})
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create invite code"})
				return
			}

			auditAdminAction(r.Context(), opts.AuditLogs, adminAuditInput{
				ActorUserID: headerPtr(r, "X-Admin-User-ID"),
				EntityType:  "invite_code",
				EntityID:    &created.ID,
				Action:      "admin.invite_code.create",
				Details: map[string]any{
					"code":            created.Code,
					"max_activations": created.MaxActivations,
				},
			})

			respondJSON(w, http.StatusCreated, map[string]any{"item": created})
		})

		ar.Post("/invite-codes/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
			if opts.InviteCodes == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "invite code repository not configured"})
				return
			}

			id := strings.TrimSpace(chi.URLParam(r, "id"))
			if id == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
				return
			}

			revoked, err := opts.InviteCodes.RevokeByID(r.Context(), id)
			if err != nil {
				respondJSON(w, http.StatusNotFound, map[string]any{"error": "invite code not found"})
				return
			}

			auditAdminAction(r.Context(), opts.AuditLogs, adminAuditInput{
				ActorUserID: headerPtr(r, "X-Admin-User-ID"),
				EntityType:  "invite_code",
				EntityID:    &revoked.ID,
				Action:      "admin.invite_code.revoke",
				Details: map[string]any{
					"status": revoked.Status,
				},
			})

			respondJSON(w, http.StatusOK, map[string]any{"item": revoked})
		})
	})

	return r
}

func adminAuthMiddleware(headerName, secret string) func(http.Handler) http.Handler {
	header := strings.TrimSpace(headerName)
	if header == "" {
		header = "X-Admin-Secret"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(secret) == "" {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "admin auth secret is not configured"})
				return
			}
			if r.Header.Get(header) != secret {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func generateInviteCode() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%04d", time.Now().UnixNano()%10_000)
	}
	n := int(b[0])<<8 | int(b[1])
	return fmt.Sprintf("%04d", n%10_000)
}

type adminAuditInput struct {
	ActorUserID *string
	EntityType  string
	EntityID    *string
	Action      string
	Details     map[string]any
}

func auditAdminAction(ctx context.Context, repo AuditLogger, in adminAuditInput) {
	if repo == nil {
		return
	}
	detailsJSON := "{}"
	if len(in.Details) > 0 {
		if b, err := json.Marshal(in.Details); err == nil {
			detailsJSON = string(b)
		}
	}
	_, _ = repo.Create(ctx, audit.CreateParams{
		ActorUserID: in.ActorUserID,
		EntityType:  in.EntityType,
		EntityID:    in.EntityID,
		Action:      in.Action,
		DetailsJSON: detailsJSON,
	})
}

func headerPtr(r *http.Request, key string) *string {
	v := strings.TrimSpace(r.Header.Get(key))
	if v == "" {
		return nil
	}
	return &v
}

func respondJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
