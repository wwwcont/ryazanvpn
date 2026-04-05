package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/wwwcont/ryazanvpn/shared/contracts/nodeapi"
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
		nodeID:  cfg.NodeID,
		token:   cfg.NodeToken,
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
	nodeID  string
	token   string
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
	req.Header.Set(nodeapi.HeaderProtocolVersion, nodeapi.CurrentProtocolVersion)
	req.Header.Set("X-Request-Id", newRequestID())
	req.Header.Set("X-Node-Id", c.nodeID)
	req.Header.Set("X-Node-Token", c.token)
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
	path := c.baseURL + "/nodes/desired-state?node_id=" + nodeID
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(auth.HeaderTimestamp, ts)
	req.Header.Set(auth.HeaderSignature, auth.Sign(c.secret, ts, nil))
	req.Header.Set(nodeapi.HeaderProtocolVersion, nodeapi.CurrentProtocolVersion)
	req.Header.Set("X-Node-Id", nodeID)
	req.Header.Set("X-Node-Token", nodeToken)
	req.Header.Set("X-Request-Id", newRequestID())
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
		AccessID            string         `json:"access_id"`
		Protocol            string         `json:"protocol"`
		PeerPublicKey       string         `json:"peer_public_key"`
		AssignedIP          string         `json:"assigned_ip"`
		PersistentKeepalive int            `json:"persistent_keepalive"`
		EndpointParams      map[string]any `json:"endpoint_params"`
	} `json:"peers"`
}

func runNodeControlLoop(ctx context.Context, logger *slog.Logger, cfg app.Config, rt runtime.VPNRuntime, cp controlPlaneClient) {
	registerPayload := map[string]any{
		"node_id":        cfg.NodeID,
		"node_name":      cfg.NodeName,
		"agent_base_url": cfg.NodeAgentBaseURL,
		"region":         cfg.NodeRegion,
		"public_ip":      cfg.NodePublicIP,
		"protocols":      cfg.NodeProtocolsSupported,
		"capacity":       cfg.NodeCapacity,
	}
	registered := false
	register := func() {
		if err := cp.post(ctx, "/nodes/register", registerPayload); err != nil {
			logger.Error("node register failed", slog.Any("error", err))
			return
		}
		registered = true
	}
	register()

	interval := cfg.NodeHeartbeatInterval
	if interval < 30*time.Second || interval > 60*time.Second {
		interval = 45 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	reconcile := func() {
		if !registered {
			register()
		}
		healthStatus := "active"
		availableProtocols := make([]string, 0, len(cfg.NodeProtocolsSupported))
		healthCtx, cancel := context.WithTimeout(ctx, cfg.RuntimeExecTimeout)
		if err := rt.Health(healthCtx); err != nil {
			healthStatus = "unhealthy"
		} else {
			availableProtocols = append(availableProtocols, "wireguard")
		}
		if checkXrayContainer(healthCtx, cfg.DockerBinaryPath, cfg.XrayContainerName) == nil {
			availableProtocols = append(availableProtocols, "xray")
		} else if containsProtocol(cfg.NodeProtocolsSupported, "xray") {
			healthStatus = "unhealthy"
		}
		cancel()
		if len(availableProtocols) == 0 {
			availableProtocols = []string{"wireguard"}
		}
		if err := cp.post(ctx, "/nodes/heartbeat", map[string]any{"node_id": cfg.NodeID, "status": healthStatus, "protocols": availableProtocols}); err != nil {
			logger.Error("node heartbeat failed", slog.Any("error", err))
		}

		desired, err := cp.getDesired(ctx, cfg.NodeID, cfg.NodeToken)
		if err != nil {
			logger.Error("desired state fetch failed", slog.Any("error", err))
			registered = false
			return
		}
		results := reconcileRuntime(ctx, logger, rt, desired.Peers)
		if err := cp.post(ctx, "/nodes/apply", map[string]any{"node_id": cfg.NodeID, "results": results, "at": time.Now().UTC()}); err != nil {
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

func checkXrayContainer(ctx context.Context, dockerBin, container string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("xray container is not configured")
	}
	bin := strings.TrimSpace(dockerBin)
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.CommandContext(ctx, bin, "inspect", "--type", "container", container)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray inspect failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func containsProtocol(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
	}
	return "req-" + hex.EncodeToString(b[:]) + "-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}

func reconcileRuntime(ctx context.Context, logger *slog.Logger, rt runtime.VPNRuntime, desired []struct {
	AccessID            string         `json:"access_id"`
	Protocol            string         `json:"protocol"`
	PeerPublicKey       string         `json:"peer_public_key"`
	AssignedIP          string         `json:"assigned_ip"`
	PersistentKeepalive int            `json:"persistent_keepalive"`
	EndpointParams      map[string]any `json:"endpoint_params"`
}) []map[string]any {
	stats, err := rt.ListPeerStats(ctx)
	if err != nil {
		logger.Error("reconcile list runtime peers failed", slog.Any("error", err))
		return []map[string]any{{"error": err.Error()}}
	}
	desiredByAccess := make(map[string]struct {
		Protocol       string
		PeerPublicKey  string
		AssignedIP     string
		Keepalive      int
		EndpointParams map[string]any
	}, len(desired))
	desiredFingerprints := make(map[string]struct{}, len(desired))
	for _, d := range desired {
		desiredByAccess[d.AccessID] = struct {
			Protocol       string
			PeerPublicKey  string
			AssignedIP     string
			Keepalive      int
			EndpointParams map[string]any
		}{Protocol: d.Protocol, PeerPublicKey: d.PeerPublicKey, AssignedIP: d.AssignedIP, Keepalive: d.PersistentKeepalive, EndpointParams: d.EndpointParams}
		desiredFingerprints[peerFingerprint(d.Protocol, d.PeerPublicKey, d.AssignedIP)] = struct{}{}
	}
	runtimeByAccess := make(map[string]runtime.PeerStat, len(stats))
	orphanRuntimePeers := make([]runtime.PeerStat, 0, len(stats))
	for _, s := range stats {
		if strings.TrimSpace(s.DeviceAccessID) != "" {
			runtimeByAccess[s.DeviceAccessID] = s
			continue
		}
		orphanRuntimePeers = append(orphanRuntimePeers, s)
	}
	results := make([]map[string]any, 0)
	for accessID, d := range desiredByAccess {
		cur, exists := runtimeByAccess[accessID]
		isXray := strings.EqualFold(strings.TrimSpace(d.Protocol), "xray")
		if exists && cur.PeerPublicKey == d.PeerPublicKey && cur.AllowedIP == d.AssignedIP && !isXray {
			results = append(results, map[string]any{"access_id": accessID, "action": "noop"})
			continue
		}
		if exists && isXray && cur.PeerPublicKey != "" && cur.PeerPublicKey != d.PeerPublicKey {
			logger.Warn("xray.runtime.state.mismatch",
				slog.String("access_id", accessID),
				slog.String("runtime_peer_public_key", cur.PeerPublicKey),
				slog.String("desired_peer_public_key", d.PeerPublicKey),
			)
		}
		opID := fmt.Sprintf("reconcile-apply-%s-%d", accessID, time.Now().UTC().UnixNano())
		_, err := rt.ApplyPeer(ctx, runtime.PeerOperationRequest{
			OperationID:    opID,
			DeviceAccessID: accessID,
			Protocol:       d.Protocol,
			PeerPublicKey:  d.PeerPublicKey,
			AssignedIP:     d.AssignedIP,
			Keepalive:      d.Keepalive,
			EndpointMeta:   endpointParamsToStringMap(d.EndpointParams),
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
	for _, cur := range orphanRuntimePeers {
		if _, wanted := desiredFingerprints[peerFingerprint(cur.Protocol, cur.PeerPublicKey, cur.AllowedIP)]; wanted {
			continue
		}
		protocol := strings.TrimSpace(cur.Protocol)
		if protocol == "" {
			continue
		}
		opID := fmt.Sprintf("reconcile-revoke-orphan-%d", time.Now().UTC().UnixNano())
		_, err := rt.RevokePeer(ctx, runtime.PeerOperationRequest{
			OperationID:    opID,
			DeviceAccessID: valueOrFallback(strings.TrimSpace(cur.DeviceAccessID), "orphan"),
			Protocol:       protocol,
			PeerPublicKey:  strings.TrimSpace(cur.PeerPublicKey),
			AssignedIP:     strings.TrimSpace(cur.AllowedIP),
			Keepalive:      25,
		})
		if err != nil {
			logger.Error("reconcile revoke orphan failed", slog.String("protocol", protocol), slog.String("peer_public_key", cur.PeerPublicKey), slog.String("allowed_ip", cur.AllowedIP), slog.Any("error", err))
			results = append(results, map[string]any{"operation_id": opID, "action": "revoke_orphan", "status": "error", "error": err.Error(), "protocol": protocol, "peer_public_key": cur.PeerPublicKey})
			continue
		}
		logger.Info("reconcile revoke orphan peer", slog.String("protocol", protocol), slog.String("peer_public_key", cur.PeerPublicKey), slog.String("allowed_ip", cur.AllowedIP))
		results = append(results, map[string]any{"operation_id": opID, "action": "revoke_orphan", "status": "ok", "protocol": protocol, "peer_public_key": cur.PeerPublicKey})
	}
	return results
}

func peerFingerprint(protocol, peerPublicKey, allowedIP string) string {
	return strings.ToLower(strings.TrimSpace(protocol)) + "|" + strings.TrimSpace(peerPublicKey) + "|" + normalizeIPForCompare(allowedIP)
}

func normalizeIPForCompare(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '/'); i > 0 {
		raw = raw[:i]
	}
	return raw
}

func valueOrFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func endpointParamsToStringMap(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(fmt.Sprint(v))
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
			logger.Warn("runtime health check failed on startup; continuing in degraded mode", slog.Any("error", err))
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
			WorkDir:           cfg.RuntimeWorkDir,
			DockerBinaryPath:  resolved,
			ContainerName:     cfg.AmneziaContainerName,
			InterfaceName:     cfg.AmneziaInterfaceName,
			ExpectedPort:      cfg.AmneziaPort,
			XrayContainer:     cfg.XrayContainerName,
			XrayConfigPath:    cfg.XrayConfigPath,
			XrayAPIAddress:    cfg.XrayAPIAddress,
			XrayAPIInboundTag: cfg.XrayAPIInboundTag,
			XrayClientFlow:    cfg.XrayClientFlow,
			CommandTimeout:    cfg.RuntimeExecTimeout,
		}, shell.NewOSExecutor(logger))
		healthCtx, cancel := context.WithTimeout(context.Background(), cfg.RuntimeExecTimeout)
		defer cancel()
		if err := rt.Health(healthCtx); err != nil {
			logger.Warn("runtime health check failed on startup; continuing in degraded mode", slog.Any("error", err))
		}
		return rt, nil
	}
	if adapter == "mock" || adapter == "" {
		logger.Info("runtime adapter selected", slog.String("adapter", "mock"))
		return runtime.NewMockRuntime(logger), nil
	}

	return nil, errors.New("unsupported runtime adapter: " + adapter)
}
