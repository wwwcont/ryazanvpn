package httpcontrol

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10)
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1_000_000)
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
