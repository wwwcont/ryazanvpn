package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type AmneziaDockerRuntimeConfig struct {
	WorkDir          string
	DockerBinaryPath string
	ContainerName    string
	InterfaceName    string
	ExpectedPort     string
	XrayContainer    string
	XrayConfigPath   string
	CommandTimeout   time.Duration
}

type AmneziaDockerRuntime struct {
	logger *slog.Logger
	cfg    AmneziaDockerRuntimeConfig
	exec   shell.CommandExecutor

	mu         sync.Mutex
	operations map[string]operationRecord
	peersByKey map[string]PeerState
}

func NewAmneziaDockerRuntime(logger *slog.Logger, cfg AmneziaDockerRuntimeConfig, executor shell.CommandExecutor) *AmneziaDockerRuntime {
	return &AmneziaDockerRuntime{
		logger:     logger,
		cfg:        cfg,
		exec:       executor,
		operations: make(map[string]operationRecord),
		peersByKey: make(map[string]PeerState),
	}
}

func (r *AmneziaDockerRuntime) Health(ctx context.Context) error {
	if r.exec == nil {
		return errors.New("shell executor is nil")
	}
	r.resolveDockerBinaryPath()
	if strings.TrimSpace(r.cfg.DockerBinaryPath) == "" {
		return errors.New("docker binary path is empty")
	}
	if strings.TrimSpace(r.cfg.ContainerName) == "" {
		return errors.New("amnezia container name is empty")
	}
	if strings.TrimSpace(r.cfg.InterfaceName) == "" {
		return errors.New("amnezia interface name is empty")
	}

	if err := r.execOK(ctx, []string{"version", "--format", "{{.Client.Version}}"}, "runtime.amnezia_docker.health.docker_version"); err != nil {
		return err
	}
	if err := r.execOK(ctx, []string{"inspect", "--type", "container", r.cfg.ContainerName}, "runtime.amnezia_docker.health.container_inspect"); err != nil {
		return err
	}
	if err := r.execOK(ctx, []string{"exec", r.cfg.ContainerName, "awg", "show", r.cfg.InterfaceName}, "runtime.amnezia_docker.health.awg_show"); err != nil {
		return err
	}
	if err := r.logAmneziaListenPort(ctx); err != nil {
		r.log("amnezia.server_listen_port.actual", slog.String("error", err.Error()))
	}

	r.log("runtime.amnezia_docker.health.ok",
		slog.String("container", r.cfg.ContainerName),
		slog.String("interface", r.cfg.InterfaceName),
	)
	return nil
}

func (r *AmneziaDockerRuntime) resolveDockerBinaryPath() {
	current := strings.TrimSpace(r.cfg.DockerBinaryPath)
	if current != "" && strings.ContainsRune(current, '/') {
		if path, err := exec.LookPath(current); err == nil {
			r.cfg.DockerBinaryPath = path
			return
		}
	}
	if path, err := exec.LookPath("docker"); err == nil {
		if current != path {
			r.log("runtime.amnezia_docker.health.docker_path_resolved",
				slog.String("configured", current),
				slog.String("resolved", path),
			)
		}
		r.cfg.DockerBinaryPath = path
	}
}

func (r *AmneziaDockerRuntime) ApplyPeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error) {
	if err := validatePeerRequest(req); err != nil {
		return OperationResult{}, err
	}
	if strings.EqualFold(strings.TrimSpace(req.Protocol), "xray") {
		if err := r.applyXrayClient(ctx, req); err != nil {
			return OperationResult{}, err
		}
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
	}

	r.mu.Lock()
	if rec, ok := r.operations[req.OperationID]; ok {
		r.mu.Unlock()
		if rec.action != "apply" || rec.deviceAccessID != req.DeviceAccessID {
			return OperationResult{}, ErrOperationConflict
		}
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: true}, nil
	}
	r.operations[req.OperationID] = operationRecord{action: "apply", deviceAccessID: req.DeviceAccessID}
	r.mu.Unlock()

	assignedIP, err := normalizeAssignedIP(req.AssignedIP)
	if err != nil {
		return OperationResult{}, err
	}

	args := []string{"exec", r.cfg.ContainerName, "awg", "set", r.cfg.InterfaceName, "peer", req.PeerPublicKey}
	cleanupPath := ""
	if psk := strings.TrimSpace(req.EndpointMeta["preshared_key"]); psk != "" {
		cleanupPath, err = r.writePresharedKeyFile(ctx, psk)
		if err != nil {
			return OperationResult{}, err
		}
		defer r.removeContainerFile(context.Background(), cleanupPath)
		args = append(args, "preshared-key", cleanupPath)
	}
	args = append(args, "allowed-ips", assignedIP+"/32")

	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return OperationResult{}, fmt.Errorf("apply peer command failed: %w", err)
	}
	if res.ExitCode != 0 {
		return OperationResult{}, fmt.Errorf("apply peer command failed with exit_code=%d", res.ExitCode)
	}

	r.mu.Lock()
	r.peersByKey[req.PeerPublicKey] = PeerState{DeviceAccessID: req.DeviceAccessID, PeerPublicKey: req.PeerPublicKey, AssignedIP: assignedIP, Keepalive: req.Keepalive, UpdatedAt: time.Now().UTC()}
	r.mu.Unlock()

	r.log("runtime.amnezia_docker.apply_peer.server_config_written",
		slog.String("operation_id", req.OperationID),
		slog.String("device_access_id", req.DeviceAccessID),
		slog.String("peer_public_key", req.PeerPublicKey),
		slog.String("assigned_ip", assignedIP),
	)
	return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
}

func (r *AmneziaDockerRuntime) RevokePeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error) {
	if strings.TrimSpace(req.OperationID) == "" || strings.TrimSpace(req.DeviceAccessID) == "" || strings.TrimSpace(req.PeerPublicKey) == "" {
		return OperationResult{}, errors.New("operation_id, device_access_id and peer_public_key are required")
	}
	if strings.EqualFold(strings.TrimSpace(req.Protocol), "xray") {
		r.log("runtime.amnezia_docker.revoke_peer.xray.skipped", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID))
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
	}

	r.mu.Lock()
	if rec, ok := r.operations[req.OperationID]; ok {
		r.mu.Unlock()
		if rec.action != "revoke" || rec.deviceAccessID != req.DeviceAccessID {
			return OperationResult{}, ErrOperationConflict
		}
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: true}, nil
	}
	r.operations[req.OperationID] = operationRecord{action: "revoke", deviceAccessID: req.DeviceAccessID}
	r.mu.Unlock()

	args := []string{"exec", r.cfg.ContainerName, "awg", "set", r.cfg.InterfaceName, "peer", req.PeerPublicKey, "remove"}
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return OperationResult{}, fmt.Errorf("revoke peer command failed: %w", err)
	}
	if res.ExitCode != 0 {
		return OperationResult{}, fmt.Errorf("revoke peer command failed with exit_code=%d", res.ExitCode)
	}

	r.mu.Lock()
	delete(r.peersByKey, req.PeerPublicKey)
	r.mu.Unlock()

	r.log("runtime.amnezia_docker.revoke_peer.ok", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.String("peer_public_key", req.PeerPublicKey))
	return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
}

func (r *AmneziaDockerRuntime) ListPeerStats(ctx context.Context) ([]PeerStat, error) {
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.ContainerName, "awg", "show", r.cfg.InterfaceName, "dump"}, Timeout: r.commandTimeout()})
	if err != nil || res.ExitCode != 0 {
		r.mu.Lock()
		defer r.mu.Unlock()
		stats := make([]PeerStat, 0, len(r.peersByKey))
		for _, peer := range r.peersByKey {
			if strings.EqualFold(strings.TrimSpace(peer.Protocol), "xray") {
				stats = append(stats, PeerStat{
					Protocol:       "xray",
					DeviceAccessID: peer.DeviceAccessID,
					PeerPublicKey:  peer.PeerPublicKey,
					AllowedIP:      peer.AssignedIP,
				})
			}
		}
		if len(stats) > 0 {
			return stats, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list peer stats command failed: %w", err)
		}
		return nil, fmt.Errorf("list peer stats command failed with exit_code=%d", res.ExitCode)
	}
	stats, err := ParseAWGShowAllDump(res.Stdout)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range stats {
		if s, ok := r.peersByKey[stats[i].PeerPublicKey]; ok {
			stats[i].DeviceAccessID = s.DeviceAccessID
		}
	}
	return stats, nil
}

func ParseAWGShowAllDump(out string) ([]PeerStat, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	stats := make([]PeerStat, 0, len(lines))

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// первая строка dump — это описание интерфейса, а не peer
		if i == 0 {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 8 {
			continue
		}

		allowed := firstAllowedIP(parts[3])

		handshake, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid handshake timestamp %q", parts[4])
		}
		rx, err := strconv.ParseInt(parts[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rx bytes %q", parts[5])
		}
		tx, err := strconv.ParseInt(parts[6], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid tx bytes %q", parts[6])
		}

		var hs *time.Time
		if handshake > 0 {
			t := time.Unix(handshake, 0).UTC()
			hs = &t
		}

		stats = append(stats, PeerStat{
			Protocol:                "wireguard",
			PeerPublicKey:           parts[0],
			PresharedKey:            parts[1],
			Endpoint:                parts[2],
			AllowedIP:               allowed,
			LatestHandshakeUnixTime: handshake,
			RXTotalBytes:            rx,
			TXTotalBytes:            tx,
			LastHandshakeAt:         hs,
		})
	}

	return stats, nil
}

func firstAllowedIP(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	first := strings.TrimSpace(parts[0])
	if ip, _, err := net.ParseCIDR(first); err == nil {
		return ip.String()
	}
	return strings.TrimSuffix(first, "/32")
}

func (r *AmneziaDockerRuntime) execOK(ctx context.Context, args []string, op string) error {
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: exit_code=%d", op, res.ExitCode)
	}
	return nil
}

func validatePeerRequest(req PeerOperationRequest) error {
	if strings.TrimSpace(req.OperationID) == "" || strings.TrimSpace(req.DeviceAccessID) == "" || strings.TrimSpace(req.PeerPublicKey) == "" || strings.TrimSpace(req.AssignedIP) == "" {
		return errors.New("operation_id, device_access_id, peer_public_key and assigned_ip are required")
	}
	if req.Keepalive < 0 {
		return errors.New("keepalive must be non-negative")
	}
	return nil
}

func normalizeAssignedIP(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if parsed := net.ParseIP(ip); parsed != nil {
		return parsed.String(), nil
	}
	addr, network, err := net.ParseCIDR(ip)
	if err != nil {
		return "", fmt.Errorf("invalid assigned_ip %q", ip)
	}
	ones, _ := network.Mask.Size()
	if ones != 32 {
		return "", fmt.Errorf("assigned_ip must be host /32, got %q", ip)
	}
	return addr.String(), nil
}

func (r *AmneziaDockerRuntime) writePresharedKeyFile(ctx context.Context, presharedKey string) (string, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9+/=._-]+$`).MatchString(presharedKey) {
		return "", errors.New("preshared key has unsupported characters")
	}
	randomPart := make([]byte, 8)
	if _, err := rand.Read(randomPart); err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}
	containerPath := "/tmp/ryazanvpn-psk-" + hex.EncodeToString(randomPart)
	args := []string{
		"exec", r.cfg.ContainerName, "sh", "-c",
		`umask 077; printf "%s" "$1" > "$2"`,
		"--", presharedKey, containerPath,
	}
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return "", fmt.Errorf("create preshared key file: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("create preshared key file failed with exit_code=%d", res.ExitCode)
	}
	return containerPath, nil
}

func (r *AmneziaDockerRuntime) removeContainerFile(ctx context.Context, filePath string) {
	if strings.TrimSpace(filePath) == "" {
		return
	}
	_, _ = r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.ContainerName, "rm", "-f", filePath}, Timeout: r.commandTimeout()})
}

func (r *AmneziaDockerRuntime) commandTimeout() time.Duration {
	if r.cfg.CommandTimeout <= 0 {
		return 10 * time.Second
	}
	return r.cfg.CommandTimeout
}

func (r *AmneziaDockerRuntime) log(msg string, attrs ...any) {
	if r.logger == nil {
		slog.Default().Info(msg, attrs...)
		return
	}
	r.logger.Info(msg, attrs...)
}

func (r *AmneziaDockerRuntime) logAmneziaListenPort(ctx context.Context) error {
	expected := strings.TrimSpace(r.cfg.ExpectedPort)
	r.log("amnezia.server_listen_port.expected", slog.String("value", expected))
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.ContainerName, "awg", "show", r.cfg.InterfaceName, "listen-port"}, Timeout: r.commandTimeout()})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("listen-port command failed with exit_code=%d", res.ExitCode)
	}
	actual := strings.TrimSpace(res.Stdout)
	r.log("amnezia.server_listen_port.actual", slog.String("value", actual))
	return nil
}

type xrayConfig struct {
	Inbounds []struct {
		Protocol string `json:"protocol"`
		Settings struct {
			Clients []map[string]any `json:"clients"`
		} `json:"settings"`
	} `json:"inbounds"`
}

func (r *AmneziaDockerRuntime) applyXrayClient(ctx context.Context, req PeerOperationRequest) error {
	uuid := strings.TrimSpace(req.PeerPublicKey)
	if uuid == "" {
		return errors.New("xray client uuid is empty")
	}
	if strings.TrimSpace(r.cfg.XrayContainer) == "" {
		return errors.New("xray container is empty")
	}
	configPath := strings.TrimSpace(r.cfg.XrayConfigPath)
	if configPath == "" {
		configPath = "/etc/xray/config.json"
	}
	r.log("xray.client.add.start", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.String("uuid", uuid))
	out, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.XrayContainer, "cat", configPath}, Timeout: r.commandTimeout()})
	if err != nil {
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if out.ExitCode != 0 {
		err := fmt.Errorf("read xray config failed with exit_code=%d", out.ExitCode)
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	var cfg xrayConfig
	if err := json.Unmarshal([]byte(out.Stdout), &cfg); err != nil {
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	updated := false
	for i := range cfg.Inbounds {
		if !strings.EqualFold(strings.TrimSpace(cfg.Inbounds[i].Protocol), "vless") {
			continue
		}
		exists := false
		for _, c := range cfg.Inbounds[i].Settings.Clients {
			if id, ok := c["id"].(string); ok && strings.EqualFold(strings.TrimSpace(id), uuid) {
				exists = true
				break
			}
		}
		if !exists {
			cfg.Inbounds[i].Settings.Clients = append(cfg.Inbounds[i].Settings.Clients, map[string]any{"id": uuid})
			updated = true
		}
	}
	if !updated {
		r.log("xray.client.add.success", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.Bool("idempotent", true))
		return nil
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	tmpPath, err := r.writeLocalTemp(raw)
	if err != nil {
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	defer os.Remove(tmpPath)
	r.log("xray.config.write.start", slog.String("operation_id", req.OperationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", configPath))
	if res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"cp", tmpPath, r.cfg.XrayContainer + ":" + configPath}, Timeout: r.commandTimeout()}); err != nil || res.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("docker cp xray config failed with exit_code=%d", res.ExitCode)
		}
		r.log("xray.config.write.error", slog.String("operation_id", req.OperationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", configPath), slog.String("error", err.Error()))
		r.log("xray.client.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	r.log("xray.config.write.success", slog.String("operation_id", req.OperationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", configPath))
	r.log("xray.config.reload.start", slog.String("container", r.cfg.XrayContainer))
	if res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"restart", r.cfg.XrayContainer}, Timeout: r.commandTimeout()}); err != nil || res.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("xray restart failed with exit_code=%d", res.ExitCode)
		}
		r.log("xray.config.reload.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	r.log("xray.config.reload.success", slog.String("operation_id", req.OperationID), slog.String("container", r.cfg.XrayContainer))
	r.log("xray.client.add.success", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.Bool("idempotent", false))
	return nil
}

func (r *AmneziaDockerRuntime) writeLocalTemp(content []byte) (string, error) {
	base := strings.TrimSpace(r.cfg.WorkDir)
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return "", err
	}
	name := filepath.Join(base, fmt.Sprintf("xray-config-%d.json", time.Now().UTC().UnixNano()))
	if err := os.WriteFile(name, content, 0o600); err != nil {
		return "", err
	}
	return name, nil
}
