package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/auth"
	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/infra/logging"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpnode"
)

func main() {
	cfg, err := app.LoadConfig("node-agent")
	if err != nil {
		slog.Error("load config", slog.Any("error", err))
		os.Exit(1)
	}
	if cfg.AgentHMACSecret == "" {
		slog.Error("AGENT_HMAC_SECRET is required for node-agent")
		os.Exit(1)
	}

	logger := logging.NewJSONLogger(cfg.LogLevel)
	logger.Info("starting service", slog.String("config", cfg.String()))

	vpnRuntime, runtimeErr := buildRuntime(cfg, logger)
	if runtimeErr != nil {
		logger.Error("runtime init failed; using unavailable runtime", slog.Any("error", runtimeErr))
		vpnRuntime = runtime.NewUnavailableRuntime(runtimeErr)
	}

	router := httpnode.NewRouter(httpnode.Options{
		Logger:           logger,
		ReadinessTimeout: cfg.ReadinessTimeout,
		Runtime:          vpnRuntime,
		HMACSecret:       cfg.AgentHMACSecret,
		HMACMaxSkew:      cfg.AgentHMACMaxSkew,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		errCh <- srv.ListenAndServe()
	}()

	agentCtl := controlPlaneClient{
		baseURL: strings.TrimRight(cfg.ControlPlaneBaseURL, "/"),
		secret:  []byte(cfg.AgentHMACSecret),
		client:  &http.Client{Timeout: cfg.NodeAgentTimeout},
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	defer loopCancel()
	if cfg.NodeID != "" && cfg.NodeToken != "" {
		go runNodeControlLoop(loopCtx, logger, cfg, vpnRuntime, agentCtl)
	} else {
		logger.Warn("NODE_ID or NODE_TOKEN is empty; register/heartbeat/reconcile loop disabled")
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
	case err = <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server exited", slog.Any("error", err))
			os.Exit(1)
		}
	}
	loopCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.Any("error", err))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Error("force close failed", slog.Any("error", closeErr))
		}
	}

	logger.Info("service stopped")
}

type controlPlaneClient struct {
	baseURL string
	secret  []byte
	client  *http.Client
}

func (c controlPlaneClient) post(ctx context.Context, path string, payload any) error {
	body, _ := json.Marshal(payload)
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderTimestamp, ts)
	req.Header.Set(auth.HeaderSignature, auth.Sign(c.secret, ts, body))
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("control-plane %s failed: status=%d body=%s", path, resp.StatusCode, string(raw))
	}
	return nil
}

func (c controlPlaneClient) getDesired(ctx context.Context, nodeID string, nodeToken string) (*desiredStateResponse, error) {
	path := c.baseURL + "/nodes/desired-state?node_id=" + nodeID + "&node_token=" + nodeToken
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(auth.HeaderTimestamp, ts)
	req.Header.Set(auth.HeaderSignature, auth.Sign(c.secret, ts, nil))
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("control-plane desired state failed: status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out desiredStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

type desiredStateResponse struct {
	NodeStatus string `json:"node_status"`
	Peers      []struct {
		AccessID           string         `json:"access_id"`
		Protocol           string         `json:"protocol"`
		PeerPublicKey      string         `json:"peer_public_key"`
		AssignedIP         string         `json:"assigned_ip"`
		PersistentKeepalive int           `json:"persistent_keepalive"`
		EndpointParams     map[string]any `json:"endpoint_params"`
	} `json:"peers"`
}

func runNodeControlLoop(ctx context.Context, logger *slog.Logger, cfg app.Config, rt runtime.VPNRuntime, cp controlPlaneClient) {
	registerPayload := map[string]any{
		"node_id":        cfg.NodeID,
		"node_token":     cfg.NodeToken,
		"node_name":      cfg.NodeName,
		"agent_base_url": cfg.NodeAgentBaseURL,
		"region":         cfg.NodeRegion,
		"public_ip":      cfg.NodePublicIP,
		"protocols":      cfg.NodeProtocolsSupported,
		"capacity":       cfg.NodeCapacity,
	}
	if err := cp.post(ctx, "/nodes/register", registerPayload); err != nil {
		logger.Error("node register failed", slog.Any("error", err))
	}

	interval := cfg.NodeHeartbeatInterval
	if interval < 30*time.Second || interval > 60*time.Second {
		interval = 45 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	reconcile := func() {
		healthStatus := "active"
		healthCtx, cancel := context.WithTimeout(ctx, cfg.RuntimeExecTimeout)
		if err := rt.Health(healthCtx); err != nil {
			healthStatus = "unhealthy"
		}
		cancel()
		if err := cp.post(ctx, "/nodes/heartbeat", map[string]any{"node_id": cfg.NodeID, "node_token": cfg.NodeToken, "status": healthStatus, "protocols": cfg.NodeProtocolsSupported}); err != nil {
			logger.Error("node heartbeat failed", slog.Any("error", err))
		}

		desired, err := cp.getDesired(ctx, cfg.NodeID, cfg.NodeToken)
		if err != nil {
			logger.Error("desired state fetch failed", slog.Any("error", err))
			return
		}
		results := reconcileRuntime(ctx, logger, rt, desired.Peers)
		if err := cp.post(ctx, "/nodes/apply", map[string]any{"node_id": cfg.NodeID, "node_token": cfg.NodeToken, "results": results, "at": time.Now().UTC()}); err != nil {
			logger.Error("apply report failed", slog.Any("error", err))
		}
	}

	reconcile()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func reconcileRuntime(ctx context.Context, logger *slog.Logger, rt runtime.VPNRuntime, desired []struct {
	AccessID           string         `json:"access_id"`
	Protocol           string         `json:"protocol"`
	PeerPublicKey      string         `json:"peer_public_key"`
	AssignedIP         string         `json:"assigned_ip"`
	PersistentKeepalive int           `json:"persistent_keepalive"`
	EndpointParams     map[string]any `json:"endpoint_params"`
}) []map[string]any {
	stats, err := rt.ListPeerStats(ctx)
	if err != nil {
		logger.Error("reconcile list runtime peers failed", slog.Any("error", err))
		return []map[string]any{{"error": err.Error()}}
	}
	desiredByAccess := make(map[string]struct {
			Protocol      string
			PeerPublicKey string
			AssignedIP    string
			Keepalive     int
		}, len(desired))
	for _, d := range desired {
		desiredByAccess[d.AccessID] = struct {
			Protocol      string
			PeerPublicKey string
			AssignedIP    string
			Keepalive     int
		}{Protocol: d.Protocol, PeerPublicKey: d.PeerPublicKey, AssignedIP: d.AssignedIP, Keepalive: d.PersistentKeepalive}
	}
	runtimeByAccess := make(map[string]runtime.PeerStat, len(stats))
	for _, s := range stats {
		if strings.TrimSpace(s.DeviceAccessID) != "" {
			runtimeByAccess[s.DeviceAccessID] = s
		}
	}
	results := make([]map[string]any, 0)
	for accessID, d := range desiredByAccess {
		cur, exists := runtimeByAccess[accessID]
		if exists && cur.PeerPublicKey == d.PeerPublicKey && cur.AllowedIP == d.AssignedIP {
			results = append(results, map[string]any{"access_id": accessID, "action": "noop"})
			continue
		}
		opID := fmt.Sprintf("reconcile-apply-%s-%d", accessID, time.Now().UTC().UnixNano())
		_, err := rt.ApplyPeer(ctx, runtime.PeerOperationRequest{
			OperationID:    opID,
			DeviceAccessID: accessID,
			Protocol:       d.Protocol,
				PeerPublicKey:  d.PeerPublicKey,
			AssignedIP:     d.AssignedIP,
			Keepalive:      d.Keepalive,
		})
		if err != nil {
			logger.Error("reconcile apply failed", slog.String("access_id", accessID), slog.Any("error", err))
			results = append(results, map[string]any{"operation_id": opID, "access_id": accessID, "action": "apply", "status": "error", "error": err.Error()})
			continue
		}
		logger.Info("reconcile apply peer", slog.String("access_id", accessID))
		results = append(results, map[string]any{"operation_id": opID, "access_id": accessID, "action": "apply", "status": "ok"})
	}
	for accessID, cur := range runtimeByAccess {
		if _, wanted := desiredByAccess[accessID]; wanted {
			continue
		}
		protocol := strings.TrimSpace(cur.Protocol)
		if protocol == "" {
			logger.Error("reconcile revoke skipped: runtime peer protocol is empty", slog.String("access_id", accessID))
			results = append(results, map[string]any{"access_id": accessID, "action": "revoke", "status": "error", "error": "runtime protocol is empty"})
			continue
		}
		opID := fmt.Sprintf("reconcile-revoke-%s-%d", accessID, time.Now().UTC().UnixNano())
		_, err := rt.RevokePeer(ctx, runtime.PeerOperationRequest{
			OperationID:    opID,
			DeviceAccessID: accessID,
			Protocol:       protocol,
			PeerPublicKey:  cur.PeerPublicKey,
			AssignedIP:     cur.AllowedIP,
			Keepalive:      25,
		})
		if err != nil {
			logger.Error("reconcile revoke failed", slog.String("access_id", accessID), slog.Any("error", err))
			results = append(results, map[string]any{"operation_id": opID, "access_id": accessID, "action": "revoke", "status": "error", "error": err.Error()})
			continue
		}
		logger.Info("reconcile revoke peer", slog.String("access_id", accessID))
		results = append(results, map[string]any{"operation_id": opID, "access_id": accessID, "action": "revoke", "status": "ok"})
	}
	return results
}

func buildRuntime(cfg app.Config, logger *slog.Logger) (runtime.VPNRuntime, error) {
	adapter := strings.ToLower(strings.TrimSpace(cfg.RuntimeAdapter))
	if adapter == "shell" {
		logger.Info("runtime adapter selected", slog.String("adapter", "shell"))
		rt := runtime.NewShellRuntime(logger, runtime.ShellRuntimeConfig{
			WorkDir:         cfg.RuntimeWorkDir,
			AWGBinaryPath:   cfg.AWGBinaryPath,
			WGBinaryPath:    cfg.WGBinaryPath,
			IPBinaryPath:    cfg.IPBinaryPath,
			StatsBinaryPath: cfg.RuntimeStatsBinaryPath,
			StatsArgs:       cfg.RuntimeStatsArgs,
			CommandTimeout:  cfg.RuntimeExecTimeout,
		}, shell.NewOSExecutor(logger))

		healthCtx, cancel := context.WithTimeout(context.Background(), cfg.RuntimeExecTimeout)
		defer cancel()
		if err := rt.Health(healthCtx); err != nil {
			return nil, err
		}
		return rt, nil
	}
	if adapter == "amnezia_docker" {
		resolved := cfg.DockerBinaryPath
		found := false
		if strings.TrimSpace(resolved) != "" {
			if strings.ContainsRune(resolved, filepath.Separator) {
				if _, err := os.Stat(resolved); err == nil {
					found = true
				}
			} else if path, err := exec.LookPath(resolved); err == nil {
				resolved = path
				found = true
			}
		}
		if !found {
			if path, err := exec.LookPath("docker"); err == nil {
				resolved = path
				found = true
			}
		}
		logger.Info("runtime adapter selected",
			slog.String("adapter", "amnezia_docker"),
			slog.String("docker_binary_configured", cfg.DockerBinaryPath),
			slog.String("docker_binary_resolved", resolved),
			slog.Bool("docker_binary_found", found),
		)
		rt := runtime.NewAmneziaDockerRuntime(logger, runtime.AmneziaDockerRuntimeConfig{
			WorkDir:          cfg.RuntimeWorkDir,
			DockerBinaryPath: resolved,
			ContainerName:    cfg.AmneziaContainerName,
			InterfaceName:    cfg.AmneziaInterfaceName,
			CommandTimeout:   cfg.RuntimeExecTimeout,
		}, shell.NewOSExecutor(logger))
		healthCtx, cancel := context.WithTimeout(context.Background(), cfg.RuntimeExecTimeout)
		defer cancel()
		if err := rt.Health(healthCtx); err != nil {
			return nil, err
		}
		return rt, nil
	}
	if adapter == "mock" || adapter == "" {
		logger.Info("runtime adapter selected", slog.String("adapter", "mock"))
		return runtime.NewMockRuntime(logger), nil
	}

	return nil, errors.New("unsupported runtime adapter: " + adapter)
}
