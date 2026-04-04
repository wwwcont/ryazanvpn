package httpcontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/wwwcont/ryazanvpn/internal/agent/auth"
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
	AgentHMACSecret   string
	NodeRegisterToken string
	Finance           *app.FinanceService
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

	r.Get("/users/{id}/traffic", func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimSpace(chi.URLParam(r, "id"))
		if uid == "" {
			respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		type row struct {
			Protocol   string
			TotalBytes int64
		}
		rows, err := opts.PG.Query(r.Context(), `
SELECT tup.protocol, COALESCE(SUM(tup.total_bytes),0)
FROM traffic_usage_daily_protocol tup
JOIN devices d ON d.id = tup.device_id
WHERE d.user_id = $1
GROUP BY tup.protocol
ORDER BY tup.protocol`, uid)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load protocol breakdown"})
			return
		}
		defer rows.Close()
		breakdown := make([]map[string]any, 0)
		var total int64
		for rows.Next() {
			var item row
			if err := rows.Scan(&item.Protocol, &item.TotalBytes); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan breakdown"})
				return
			}
			total += item.TotalBytes
			breakdown = append(breakdown, map[string]any{"protocol": item.Protocol, "total_bytes": item.TotalBytes})
		}

		monthlyRows, err := opts.PG.Query(r.Context(), `
SELECT to_char(tum.usage_month, 'YYYY-MM') AS month, tum.protocol, COALESCE(SUM(tum.total_bytes),0)
FROM traffic_usage_monthly tum
JOIN devices d ON d.id = tum.device_id
WHERE d.user_id = $1
GROUP BY month, tum.protocol
ORDER BY month DESC, tum.protocol`, uid)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load monthly breakdown"})
			return
		}
		defer monthlyRows.Close()
		monthly := make([]map[string]any, 0)
		for monthlyRows.Next() {
			var month, protocol string
			var bytes int64
			if err := monthlyRows.Scan(&month, &protocol, &bytes); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan monthly breakdown"})
				return
			}
			monthly = append(monthly, map[string]any{"month": month, "protocol": protocol, "total_bytes": bytes})
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"user_id":                uid,
			"total_bytes":            total,
			"protocol_breakdown":     breakdown,
			"monthly_by_protocol":    monthly,
			"aggregation_data_model": "raw_snapshots + daily + monthly",
		})
	})

	r.Get("/users/{id}/balance", func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimSpace(chi.URLParam(r, "id"))
		if uid == "" {
			respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		var balance int64
		var lastCharge *time.Time
		if err := opts.PG.QueryRow(r.Context(), `SELECT balance_kopecks, last_charge_at FROM users WHERE id = $1`, uid).Scan(&balance, &lastCharge); err != nil {
			respondJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"user_id": uid, "balance_kopecks": balance, "last_charge_at": lastCharge})
	})

	r.Get("/users/{id}/ledger", func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimSpace(chi.URLParam(r, "id"))
		if uid == "" {
			respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		rows, err := opts.PG.Query(r.Context(), `SELECT id::text, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata, created_at FROM user_ledger WHERE user_id = $1 ORDER BY created_at DESC LIMIT 200`, uid)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load ledger"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, op string
			var amount, balanceAfter int64
			var reference *string
			var metadata map[string]any
			var createdAt time.Time
			if err := rows.Scan(&id, &op, &amount, &balanceAfter, &reference, &metadata, &createdAt); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan ledger"})
				return
			}
			items = append(items, map[string]any{
				"id":                    id,
				"operation_type":        op,
				"amount_kopecks":        amount,
				"balance_after_kopecks": balanceAfter,
				"reference":             reference,
				"metadata":              metadata,
				"created_at":            createdAt,
			})
		}
		respondJSON(w, http.StatusOK, map[string]any{"user_id": uid, "items": items})
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

	r.Route("/nodes", func(nr chi.Router) {
		nr.Use(nodeAgentAuthMiddleware(opts.AgentHMACSecret, opts.Logger, 5*time.Minute))

		nr.Post("/register", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				NodeID       string   `json:"node_id"`
				NodeToken    string   `json:"node_token"`
				NodeName     string   `json:"node_name"`
				AgentBaseURL string   `json:"agent_base_url"`
				Region       string   `json:"region"`
				PublicIP     string   `json:"public_ip"`
				Protocols    []string `json:"protocols"`
				Capacity     int      `json:"capacity"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
				return
			}
			if opts.NodeRegisterToken != "" && strings.TrimSpace(req.NodeToken) != strings.TrimSpace(opts.NodeRegisterToken) {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid node token"})
				return
			}
			nodeID := strings.TrimSpace(req.NodeID)
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			nodeName := strings.TrimSpace(req.NodeName)
			if nodeName == "" {
				nodeName = "node-" + nodeID
			}
			region := strings.TrimSpace(req.Region)
			if region == "" {
				region = "single-server"
			}
			agentBaseURL := strings.TrimSpace(req.AgentBaseURL)
			if agentBaseURL == "" {
				agentBaseURL = "http://node-agent:8081"
			}
			publicIP := strings.TrimSpace(req.PublicIP)
			if publicIP == "" {
				publicIP = "127.0.0.1"
			}
			capacity := req.Capacity
			if capacity <= 0 {
				capacity = 40
			}
			metadata := map[string]any{"protocols_supported": req.Protocols}
			if publicIP != "" {
				metadata["public_ip"] = publicIP
			}
			rawMeta, _ := json.Marshal(metadata)
			_, err := opts.PG.Exec(r.Context(), `
INSERT INTO vpn_nodes (
	id, name, region, status, user_capacity, agent_base_url, vpn_endpoint,
	vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr,
	runtime_metadata, last_seen_at, created_at, updated_at
) VALUES (
	$1, $2, $3, 'active', $4, $5, $6 || ':41475', $6, 41475,
	'', '10.8.1.0/24', $7::jsonb, NOW(), NOW(), NOW()
)
ON CONFLICT (id) DO UPDATE SET
	name=COALESCE(NULLIF(EXCLUDED.name,''), vpn_nodes.name),
	region=COALESCE(NULLIF(EXCLUDED.region,''), vpn_nodes.region),
	user_capacity=CASE WHEN EXCLUDED.user_capacity > 0 THEN EXCLUDED.user_capacity ELSE vpn_nodes.user_capacity END,
	agent_base_url=COALESCE(NULLIF(EXCLUDED.agent_base_url,''), vpn_nodes.agent_base_url),
	runtime_metadata=EXCLUDED.runtime_metadata,
	status='active',
	last_seen_at=NOW(),
	updated_at=NOW()`, nodeID, nodeName, region, capacity, agentBaseURL, publicIP, string(rawMeta))
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "register failed"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		})

		nr.Post("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				NodeID    string   `json:"node_id"`
				NodeToken string   `json:"node_token"`
				Status    string   `json:"status"`
				Protocols []string `json:"protocols"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
				return
			}
			if opts.NodeRegisterToken != "" && strings.TrimSpace(req.NodeToken) != strings.TrimSpace(opts.NodeRegisterToken) {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid node token"})
				return
			}
			nodeID := strings.TrimSpace(req.NodeID)
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			status := "active"
			if strings.EqualFold(strings.TrimSpace(req.Status), "unhealthy") {
				status = "down"
			}
			metadata := map[string]any{"protocols_supported": req.Protocols}
			rawMeta, _ := json.Marshal(metadata)
			_, err := opts.PG.Exec(r.Context(), `UPDATE vpn_nodes SET status=$2, runtime_metadata=$3::jsonb, last_seen_at=NOW(), updated_at=NOW() WHERE id=$1`, nodeID, status, string(rawMeta))
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "heartbeat failed"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		})

		nr.Get("/desired-state", func(w http.ResponseWriter, r *http.Request) {
			nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
			nodeToken := strings.TrimSpace(r.URL.Query().Get("node_token"))
			if opts.NodeRegisterToken != "" && nodeToken != strings.TrimSpace(opts.NodeRegisterToken) {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid node token"})
				return
			}
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			var nodeStatus string
			var runtimeMeta map[string]any
			if err := opts.PG.QueryRow(r.Context(), `SELECT status, runtime_metadata FROM vpn_nodes WHERE id=$1`, nodeID).Scan(&nodeStatus, &runtimeMeta); err != nil {
				respondJSON(w, http.StatusNotFound, map[string]any{"error": "node not found"})
				return
			}
			rows, err := opts.PG.Query(r.Context(), `
SELECT
	da.id::text,
	d.user_id::text,
	d.id::text,
	da.protocol,
	d.public_key,
	da.assigned_ip::text
FROM device_accesses da
JOIN devices d ON d.id=da.device_id
JOIN users u ON u.id=d.user_id
WHERE da.vpn_node_id=$1
  AND da.status='active'
  AND u.status='active'`, nodeID)
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load desired peers"})
				return
			}
			defer rows.Close()
			items := make([]map[string]any, 0)
			for rows.Next() {
				var accessID, userID, deviceID, protocol, publicKey string
				var assignedIP *string
				if err := rows.Scan(&accessID, &userID, &deviceID, &protocol, &publicKey, &assignedIP); err != nil {
					respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "scan failed"})
					return
				}
				assigned := ""
				if assignedIP != nil {
					assigned = *assignedIP
				}
				peerID := publicKey
				if strings.EqualFold(strings.TrimSpace(protocol), "xray") {
					peerID = app.DeterministicXrayUUIDFromSeed(accessID)
				}
				items = append(items, map[string]any{
					"access_id":            accessID,
					"user_id":              userID,
					"device_id":            deviceID,
					"protocol":             protocol,
					"peer_public_key":      peerID,
					"preshared_key":        nil,
					"assigned_ip":          assigned,
					"endpoint_params":      map[string]any{},
					"persistent_keepalive": 25,
					"node_id":              nodeID,
					"desired_state":        "present",
				})
			}
			nodeState := "active"
			if nodeStatus == "down" {
				nodeState = "unhealthy"
			}
			respondJSON(w, http.StatusOK, map[string]any{"node_status": nodeState, "runtime_metadata": runtimeMeta, "peers": items})
		})

		nr.Post("/apply", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				NodeID    string `json:"node_id"`
				NodeToken string `json:"node_token"`
				Results   []struct {
					OperationID string `json:"operation_id"`
					AccessID    string `json:"access_id"`
					Action      string `json:"action"`
					Status      string `json:"status"`
					Error       string `json:"error"`
				} `json:"results"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
				return
			}
			if opts.NodeRegisterToken != "" {
				if strings.TrimSpace(req.NodeToken) != strings.TrimSpace(opts.NodeRegisterToken) {
					respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid node token"})
					return
				}
			}
			for _, item := range req.Results {
				status := "failed"
				if strings.EqualFold(strings.TrimSpace(item.Status), "ok") || strings.EqualFold(strings.TrimSpace(item.Status), "success") {
					status = "success"
				}
				payload := map[string]any{
					"access_id": item.AccessID,
					"action":    item.Action,
					"status":    item.Status,
					"error":     item.Error,
				}
				rawPayload, _ := json.Marshal(payload)
				var opID *string
				if strings.TrimSpace(item.OperationID) != "" {
					v := strings.TrimSpace(item.OperationID)
					opID = &v
				}
				if _, err := opts.PG.Exec(r.Context(), `INSERT INTO node_apply_reports (node_id, operation_id, status, error_message, payload) VALUES ($1, $2, $3, NULLIF($4,''), $5::jsonb)`, req.NodeID, opID, status, item.Error, string(rawPayload)); err != nil {
					respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to persist apply report"})
					return
				}
				if opID != nil {
					targetStatus := "failed"
					if status == "success" {
						targetStatus = "applied"
					}
					if _, err := opts.PG.Exec(r.Context(), `UPDATE node_operations SET status = $2, error_message = NULLIF($3,''), finished_at = NOW(), updated_at = NOW() WHERE id = $1`, *opID, targetStatus, item.Error); err != nil {
						respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to update operation status"})
						return
					}
				}
			}
			if opts.Logger != nil {
				opts.Logger.Info("node apply report received", slog.Any("payload", req))
			}
			respondJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
		})
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

		ar.Post("/users/{id}/payment", func(w http.ResponseWriter, r *http.Request) {
			if opts.Finance == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "finance service not configured"})
				return
			}
			uid := strings.TrimSpace(chi.URLParam(r, "id"))
			var req struct {
				AmountKopecks int64  `json:"amount_kopecks"`
				Reference     string `json:"reference"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountKopecks <= 0 {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid payload"})
				return
			}
			if err := opts.Finance.AddPayment(r.Context(), uid, req.AmountKopecks, req.Reference); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to add payment"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		})

		ar.Post("/users/{id}/manual-adjustment", func(w http.ResponseWriter, r *http.Request) {
			if opts.Finance == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "finance service not configured"})
				return
			}
			uid := strings.TrimSpace(chi.URLParam(r, "id"))
			var req struct {
				AmountKopecks int64  `json:"amount_kopecks"`
				Reference     string `json:"reference"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountKopecks == 0 {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid payload"})
				return
			}
			if err := opts.Finance.AddManualAdjustment(r.Context(), uid, req.AmountKopecks, req.Reference); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to apply adjustment"})
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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

func nodeAgentAuthMiddleware(secret string, logger *slog.Logger, maxSkew time.Duration) func(http.Handler) http.Handler {
	key := []byte(strings.TrimSpace(secret))
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tsRaw := r.Header.Get(auth.HeaderTimestamp)
			sigHex := r.Header.Get(auth.HeaderSignature)
			if tsRaw == "" || sigHex == "" {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing auth headers"})
				return
			}
			ts, err := strconv.ParseInt(tsRaw, 10, 64)
			if err != nil {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid timestamp"})
				return
			}
			now := time.Now().UTC()
			reqTime := time.Unix(ts, 0).UTC()
			if reqTime.Before(now.Add(-maxSkew)) || reqTime.After(now.Add(maxSkew)) {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "timestamp skew exceeded"})
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot read request body"})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if !auth.Verify(key, tsRaw, body, sigHex) {
				logger.Warn("invalid signature for node endpoint", slog.String("path", r.URL.Path))
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid signature"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
