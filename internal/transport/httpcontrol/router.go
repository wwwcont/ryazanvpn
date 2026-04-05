package httpcontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	"github.com/wwwcont/ryazanvpn/internal/infra/oplog"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpcommon"
	"github.com/wwwcont/ryazanvpn/shared/contracts/nodeapi"
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
	Logger                   *slog.Logger
	PG                       *pgxpool.Pool
	RedisClient              *redis.Client
	ReadinessTimeout         time.Duration
	DownloadUC               *app.DownloadDeviceConfigByToken
	AdminSecret              string
	AdminSecretHeader        string
	Nodes                    NodeAdminReader
	Users                    UserAdminReader
	Devices                  DeviceAdminReader
	InviteCodes              InviteCodeAdminRepo
	AuditLogs                AuditLogger
	TelegramWebhook          http.Handler
	AgentHMACSecret          string
	NodeRegisterToken        string
	Finance                  *app.FinanceService
	NodeLinkCapacityBPS      int64
	NodeThroughputSampleStep time.Duration
	NodeThroughputRetention  time.Duration
	NodeRateLimitPerMinute   int
	AdminRateLimitPerMinute  int
	DailyChargeKopecks       int64
	OpsLog                   *oplog.Store
}

var inviteCodePattern = regexp.MustCompile(`^\d{4}$`)

func NewRouter(opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(httpcommon.AccessLogMiddleware(opts.Logger))
	r.Use(operationLogMiddleware(opts.OpsLog, "control-plane"))

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
			var currentBps float64
			var medianBps1h float64
			var p95Bps1h float64
			var peakBps24h float64
			var peersTotal int
			var peersResolved int
			_ = opts.PG.QueryRow(r.Context(), `
SELECT COALESCE((
    SELECT throughput_bps
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
    ORDER BY nts.captured_at DESC
    LIMIT 1
), 0),
COALESCE((
    SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY throughput_bps)
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
      AND nts.captured_at >= NOW() - INTERVAL '1 hour'
), 0),
COALESCE((
    SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY throughput_bps)
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
      AND nts.captured_at >= NOW() - INTERVAL '1 hour'
), 0),
COALESCE((
    SELECT MAX(throughput_bps)
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
      AND nts.captured_at >= NOW() - INTERVAL '24 hour'
), 0),
COALESCE((
    SELECT peers_total
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
    ORDER BY nts.captured_at DESC
    LIMIT 1
), 0),
COALESCE((
    SELECT peers_resolved
    FROM node_throughput_samples nts
    WHERE nts.vpn_node_id = $1
    ORDER BY nts.captured_at DESC
    LIMIT 1
), 0)`, id).Scan(&currentBps, &medianBps1h, &p95Bps1h, &peakBps24h, &peersTotal, &peersResolved)
			util := utilizationPercent(currentBps, opts.NodeLinkCapacityBPS)
			lowUtilAnomaly := isLowUtilizationAnomaly(currentBps, p95Bps1h, util)
			items = append(items, map[string]any{
				"id":                       id,
				"name":                     name,
				"status":                   status,
				"current_load":             load,
				"user_capacity":            capacity,
				"node_load":                nodeLoad,
				"agent_base_url":           agentURL,
				"vpn_endpoint":             endpoint,
				"last_seen_at":             lastSeen,
				"current_bps_estimate":     currentBps,
				"median_bps_1h":            medianBps1h,
				"p95_bps_1h":               p95Bps1h,
				"peak_bps_24h":             peakBps24h,
				"runtime_peers_total":      peersTotal,
				"runtime_peers_resolved":   peersResolved,
				"link_utilization_percent": util,
				"low_utilization_anomaly":  lowUtilAnomaly,
				"heartbeat_fresh_seconds":  heartbeatFreshSeconds(lastSeen),
			})
		}
		step := opts.NodeThroughputSampleStep
		if step <= 0 {
			step = time.Minute
		}
		retention := opts.NodeThroughputRetention
		if retention <= 0 {
			retention = 48 * time.Hour
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"items":               items,
			"sample_step_seconds": int64(step.Seconds()),
			"retention_seconds":   int64(retention.Seconds()),
		})
	})

	r.Get("/api/v1/metrics/nodes/top-slow", func(w http.ResponseWriter, r *http.Request) {
		rows, err := opts.PG.Query(r.Context(), `
WITH latest AS (
	SELECT DISTINCT ON (vpn_node_id) vpn_node_id, throughput_bps, captured_at
	FROM node_throughput_samples
	ORDER BY vpn_node_id, captured_at DESC
),
p95_1h AS (
	SELECT vpn_node_id, COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY throughput_bps),0) AS p95_bps_1h
	FROM node_throughput_samples
	WHERE captured_at >= NOW() - INTERVAL '1 hour'
	GROUP BY vpn_node_id
)
SELECT n.id::text, n.name, COALESCE(l.throughput_bps,0), COALESCE(p.p95_bps_1h,0), n.status, n.last_seen_at
FROM vpn_nodes n
LEFT JOIN latest l ON l.vpn_node_id = n.id
LEFT JOIN p95_1h p ON p.vpn_node_id = n.id
ORDER BY COALESCE(p.p95_bps_1h,0) DESC
LIMIT 10`)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load slow nodes"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, name, status string
			var currentBps, p95Bps float64
			var lastSeen *time.Time
			if err := rows.Scan(&id, &name, &currentBps, &p95Bps, &status, &lastSeen); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan slow nodes"})
				return
			}
			util := utilizationPercent(currentBps, opts.NodeLinkCapacityBPS)
			items = append(items, map[string]any{
				"node_id":                  id,
				"name":                     name,
				"status":                   status,
				"current_bps":              currentBps,
				"p95_bps_1h":               p95Bps,
				"link_utilization_percent": util,
				"low_utilization_anomaly":  isLowUtilizationAnomaly(currentBps, p95Bps, util),
				"heartbeat_fresh_seconds":  heartbeatFreshSeconds(lastSeen),
			})
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/api/v1/metrics/nodes/top-errors", func(w http.ResponseWriter, r *http.Request) {
		rows, err := opts.PG.Query(r.Context(), `
SELECT n.id::text, n.name,
COALESCE((
	SELECT COUNT(*)
	FROM node_apply_reports nar
	WHERE nar.node_id = n.id::text
	  AND nar.status = 'failed'
	  AND nar.created_at >= NOW() - INTERVAL '24 hour'
),0) AS apply_errors_24h,
COALESCE((
	SELECT COUNT(*)
	FROM node_operations no
	WHERE no.vpn_node_id = n.id
	  AND no.status IN ('failed', 'cancelled')
	  AND no.updated_at >= NOW() - INTERVAL '24 hour'
),0) AS operation_errors_24h
FROM vpn_nodes n
ORDER BY (COALESCE((
	SELECT COUNT(*) FROM node_apply_reports nar
	WHERE nar.node_id = n.id::text AND nar.status='failed' AND nar.created_at >= NOW() - INTERVAL '24 hour'
),0) + COALESCE((
	SELECT COUNT(*) FROM node_operations no
	WHERE no.vpn_node_id = n.id AND no.status IN ('failed', 'cancelled') AND no.updated_at >= NOW() - INTERVAL '24 hour'
),0)) DESC
LIMIT 10`)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load error nodes"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, name string
			var applyErr, opErr int64
			if err := rows.Scan(&id, &name, &applyErr, &opErr); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan error nodes"})
				return
			}
			items = append(items, map[string]any{
				"node_id":              id,
				"name":                 name,
				"apply_errors_24h":     applyErr,
				"reconcile_errors_24h": opErr,
				"total_errors_24h":     applyErr + opErr,
			})
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/api/v1/metrics/nodes/top-noisy", func(w http.ResponseWriter, r *http.Request) {
		rows, err := opts.PG.Query(r.Context(), `
SELECT n.id::text, n.name,
COALESCE((
	SELECT COUNT(*)
	FROM node_apply_reports nar
	WHERE nar.node_id = n.id::text
	  AND nar.created_at >= NOW() - INTERVAL '24 hour'
),0) AS total_apply_24h,
COALESCE((
	SELECT COUNT(*)
	FROM node_apply_reports nar
	WHERE nar.node_id = n.id::text
	  AND nar.status = 'failed'
	  AND nar.created_at >= NOW() - INTERVAL '24 hour'
),0) AS failed_apply_24h
FROM vpn_nodes n
ORDER BY failed_apply_24h DESC, total_apply_24h DESC
LIMIT 10`)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load noisy nodes"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, name string
			var totalApply, failedApply int64
			if err := rows.Scan(&id, &name, &totalApply, &failedApply); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to scan noisy nodes"})
				return
			}
			rate := 0.0
			if totalApply > 0 {
				rate = float64(failedApply) / float64(totalApply)
			}
			items = append(items, map[string]any{
				"node_id":             id,
				"name":                name,
				"total_apply_24h":     totalApply,
				"failed_apply_24h":    failedApply,
				"warn_error_rate_24h": rate,
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
		var today int64
		if err := opts.PG.QueryRow(r.Context(), `SELECT COALESCE(SUM(tud.total_bytes),0) FROM traffic_usage_daily tud JOIN devices d ON d.id=tud.device_id WHERE d.user_id = $1 AND tud.usage_date = CURRENT_DATE`, uid).Scan(&today); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load user today traffic"})
			return
		}
		var last7 int64
		if err := opts.PG.QueryRow(r.Context(), `SELECT COALESCE(SUM(tud.total_bytes),0) FROM traffic_usage_daily tud JOIN devices d ON d.id=tud.device_id WHERE d.user_id = $1 AND tud.usage_date >= CURRENT_DATE - INTERVAL '6 day'`, uid).Scan(&last7); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load user 7d traffic"})
			return
		}
		var balance int64
		var billableDevices int64
		var lastActivity *time.Time
		if err := opts.PG.QueryRow(r.Context(), `SELECT balance_kopecks FROM users WHERE id=$1`, uid).Scan(&balance); err != nil {
			respondJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
			return
		}
		_ = opts.PG.QueryRow(r.Context(), `SELECT COUNT(DISTINCT d.id) FROM devices d JOIN device_accesses da ON da.device_id=d.id WHERE d.user_id=$1 AND d.status='active' AND da.status IN ('active','suspended_nonpayment')`, uid).Scan(&billableDevices)
		_ = opts.PG.QueryRow(r.Context(), `SELECT MAX(captured_at) FROM device_traffic_snapshots s JOIN devices d ON d.id=s.device_id WHERE d.user_id=$1`, uid).Scan(&lastActivity)
		estDays := estimateDaysRemaining(balance, opts.DailyChargeKopecks, billableDevices)

		respondJSON(w, http.StatusOK, map[string]any{
			"user_id":                    uid,
			"traffic_today_bytes":        today,
			"traffic_last_7_days_bytes":  last7,
			"total_traffic_bytes":        total,
			"traffic_last_30_days_bytes": last30,
			"last_activity_at":           lastActivity,
			"estimated_days_remaining":   estDays,
			"billable_devices":           billableDevices,
			"balance_kopecks":            balance,
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
		nr.Use(rateLimitMiddleware(newWindowRateLimiter(opts.NodeRateLimitPerMinute, time.Minute), func(r *http.Request) string {
			nodeID := strings.TrimSpace(r.Header.Get("X-Node-Id"))
			if nodeID != "" {
				return "node:" + nodeID
			}
			return "ip:" + clientIP(r)
		}))
		nr.Use(nodeIdentityMiddleware(opts.NodeRegisterToken))
		nr.Use(nodeAgentAuthMiddleware(opts.AgentHMACSecret, opts.RedisClient, opts.Logger, 5*time.Minute, 5*time.Minute))

		nr.Post("/register", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				NodeID       string   `json:"node_id"`
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
			headerNodeID := nodeIDFromContext(r.Context())
			nodeID := strings.TrimSpace(req.NodeID)
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			if headerNodeID == "" || headerNodeID != nodeID {
				respondJSON(w, http.StatusForbidden, map[string]any{"error": "node_id mismatch"})
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
				Status    string   `json:"status"`
				Protocols []string `json:"protocols"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
				return
			}
			headerNodeID := nodeIDFromContext(r.Context())
			nodeID := strings.TrimSpace(req.NodeID)
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			if headerNodeID == "" || headerNodeID != nodeID {
				respondJSON(w, http.StatusForbidden, map[string]any{"error": "node_id mismatch"})
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
			headerNodeID := nodeIDFromContext(r.Context())
			nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
			if nodeID == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			if headerNodeID == "" || headerNodeID != nodeID {
				respondJSON(w, http.StatusForbidden, map[string]any{"error": "node_id mismatch"})
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
	da.assigned_ip::text,
	da.preshared_key
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
				var presharedKey *string
				if err := rows.Scan(&accessID, &userID, &deviceID, &protocol, &publicKey, &assignedIP, &presharedKey); err != nil {
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
				endpointParams := map[string]any{}
				if strings.EqualFold(strings.TrimSpace(protocol), "wireguard") && presharedKey != nil && strings.TrimSpace(*presharedKey) != "" {
					endpointParams["preshared_key"] = strings.TrimSpace(*presharedKey)
				}
				items = append(items, map[string]any{
					"access_id":            accessID,
					"user_id":              userID,
					"device_id":            deviceID,
					"protocol":             protocol,
					"peer_public_key":      peerID,
					"preshared_key":        nil,
					"assigned_ip":          assigned,
					"endpoint_params":      endpointParams,
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
				NodeID  string `json:"node_id"`
				Results []struct {
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
			headerNodeID := nodeIDFromContext(r.Context())
			if strings.TrimSpace(req.NodeID) == "" {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id is required"})
				return
			}
			if headerNodeID == "" || headerNodeID != strings.TrimSpace(req.NodeID) {
				respondJSON(w, http.StatusForbidden, map[string]any{"error": "node_id mismatch"})
				return
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
		ar.Use(rateLimitMiddleware(newWindowRateLimiter(opts.AdminRateLimitPerMinute, time.Minute), func(r *http.Request) string {
			return "ip:" + clientIP(r)
		}))
		ar.Use(adminAuthMiddleware(opts.AdminSecretHeader, opts.AdminSecret))

		ar.Get("/logs/export", func(w http.ResponseWriter, r *http.Request) {
			if opts.OpsLog == nil {
				respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "ops log export is not configured"})
				return
			}
			parseTs := func(v string) (time.Time, error) {
				if strings.TrimSpace(v) == "" {
					return time.Time{}, nil
				}
				return time.Parse(time.RFC3339, strings.TrimSpace(v))
			}
			from, err := parseTs(r.URL.Query().Get("from"))
			if err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid from timestamp, expected RFC3339"})
				return
			}
			to, err := parseTs(r.URL.Query().Get("to"))
			if err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid to timestamp, expected RFC3339"})
				return
			}
			limit := 5000
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				n, convErr := strconv.Atoi(raw)
				if convErr != nil || n <= 0 {
					respondJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid limit"})
					return
				}
				limit = n
			}
			buf := &bytes.Buffer{}
			written, err := opts.OpsLog.Export(r.Context(), buf, oplog.Filter{
				From:    from,
				To:      to,
				Service: strings.TrimSpace(r.URL.Query().Get("service")),
				NodeID:  strings.TrimSpace(r.URL.Query().Get("node_id")),
				Level:   strings.TrimSpace(r.URL.Query().Get("level")),
				Limit:   limit,
			})
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to export logs"})
				return
			}
			filename := "ops-export-" + time.Now().UTC().Format("20060102T150405Z") + ".ndjson"
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			w.Header().Set("X-Export-Count", strconv.Itoa(written))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
		})

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

type contextKey string

const (
	contextNodeIDKey contextKey = "node_id"
)

func nodeIDFromContext(ctx context.Context) string {
	raw, _ := ctx.Value(contextNodeIDKey).(string)
	return strings.TrimSpace(raw)
}

func nodeIdentityMiddleware(expectedToken string) func(http.Handler) http.Handler {
	normalized := strings.TrimSpace(expectedToken)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nodeID := strings.TrimSpace(r.Header.Get("X-Node-Id"))
			nodeToken := strings.TrimSpace(r.Header.Get("X-Node-Token"))
			if nodeID == "" || nodeToken == "" {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing node identity headers"})
				return
			}
			if normalized != "" && nodeToken != normalized {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid node token"})
				return
			}
			ctx := context.WithValue(r.Context(), contextNodeIDKey, nodeID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func nodeAgentAuthMiddleware(secret string, redisClient *redis.Client, logger *slog.Logger, maxSkew time.Duration, replayTTL time.Duration) func(http.Handler) http.Handler {
	key := []byte(strings.TrimSpace(secret))
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tsRaw := r.Header.Get(auth.HeaderTimestamp)
			sigHex := r.Header.Get(auth.HeaderSignature)
			reqID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
			protoVer := strings.TrimSpace(r.Header.Get(nodeapi.HeaderProtocolVersion))
			if tsRaw == "" || sigHex == "" {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing auth headers"})
				return
			}
			if protoVer == "" {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing protocol version"})
				return
			}
			if protoVer != nodeapi.CurrentProtocolVersion {
				respondJSON(w, http.StatusUpgradeRequired, map[string]any{"error": "unsupported protocol version"})
				return
			}
			if reqID == "" {
				respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing request id"})
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
			if redisClient != nil {
				nodeID := strings.TrimSpace(r.Header.Get("X-Node-Id"))
				if nodeID == "" {
					nodeID = "unknown"
				}
				replayKey := fmt.Sprintf("node:req:%s:%s", nodeID, reqID)
				ok, err := redisClient.SetNX(r.Context(), replayKey, "1", replayTTL).Result()
				if err != nil {
					logger.Warn("request replay check failed", slog.Any("error", err), slog.String("path", r.URL.Path))
					respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "request replay check failed"})
					return
				}
				if !ok {
					respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "replay detected"})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type windowRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]windowCounter
}

type windowCounter struct {
	count int
	reset time.Time
}

func newWindowRateLimiter(limit int, window time.Duration) *windowRateLimiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &windowRateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]windowCounter),
	}
}

func (l *windowRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.buckets[key]
	if !ok || now.After(v.reset) {
		l.buckets[key] = windowCounter{count: 1, reset: now.Add(l.window)}
		return true
	}
	if v.count >= l.limit {
		return false
	}
	v.count++
	l.buckets[key] = v
	return true
}

func rateLimitMiddleware(l *windowRateLimiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if strings.TrimSpace(key) == "" {
				key = "anonymous"
			}
			if !l.allow(key, time.Now().UTC()) {
				respondJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func operationLogMiddleware(store *oplog.Store, service string) func(http.Handler) http.Handler {
	if store == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &opStatusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now().UTC()
			next.ServeHTTP(sw, r)
			severity := "info"
			errCode := ""
			if sw.status >= 500 {
				severity = "error"
				errCode = "E_HTTP_5XX"
			} else if sw.status >= 400 {
				severity = "warn"
				errCode = "E_HTTP_4XX"
			}
			_ = store.Write(r.Context(), oplog.Record{
				Timestamp: start,
				Service:   service,
				Severity:  severity,
				ErrorCode: errCode,
				RequestID: middleware.GetReqID(r.Context()),
				NodeID:    strings.TrimSpace(r.Header.Get("X-Node-Id")),
				UserID:    strings.TrimSpace(r.Header.Get("X-User-Id")),
				DeviceID:  strings.TrimSpace(r.Header.Get("X-Device-Id")),
				Message:   r.Method + " " + r.URL.Path,
			})
		})
	}
}

type opStatusWriter struct {
	http.ResponseWriter
	status int
}

func (s *opStatusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
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
