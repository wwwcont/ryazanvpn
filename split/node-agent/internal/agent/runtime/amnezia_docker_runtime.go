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
	WorkDir           string
	DockerBinaryPath  string
	ContainerName     string
	InterfaceName     string
	ExpectedPort      string
	XrayContainer     string
	XrayConfigPath    string
	XrayAPIAddress    string
	XrayAPIInboundTag string
	XrayClientFlow    string
	CommandTimeout    time.Duration
}

type AmneziaDockerRuntime struct {
	logger *slog.Logger
	cfg    AmneziaDockerRuntimeConfig
	exec   shell.CommandExecutor

	mu                  sync.Mutex
	xrayMu              sync.Mutex
	operations          map[string]operationRecord
	peersByKey          map[string]PeerState
	pskByPeer           map[string]string
	xrayRuntimeUsers    map[string]string
	xrayContainerMarker string
}

func NewAmneziaDockerRuntime(logger *slog.Logger, cfg AmneziaDockerRuntimeConfig, executor shell.CommandExecutor) *AmneziaDockerRuntime {
	return &AmneziaDockerRuntime{
		logger:           logger,
		cfg:              cfg,
		exec:             executor,
		operations:       make(map[string]operationRecord),
		peersByKey:       make(map[string]PeerState),
		pskByPeer:        make(map[string]string),
		xrayRuntimeUsers: make(map[string]string),
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
	psk := strings.TrimSpace(req.EndpointMeta["preshared_key"])
	if psk == "" {
		r.mu.Lock()
		psk = strings.TrimSpace(r.pskByPeer[req.PeerPublicKey])
		r.mu.Unlock()
	}
	if psk != "" {
		cleanupPath, err = r.writePresharedKeyFile(ctx, psk)
		if err != nil {
			return OperationResult{}, err
		}
		defer r.removeContainerFile(context.Background(), cleanupPath)
		args = append(args, "preshared-key", cleanupPath)
		r.mu.Lock()
		r.pskByPeer[req.PeerPublicKey] = psk
		r.mu.Unlock()
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
		if err := r.revokeXrayClient(ctx, req); err != nil {
			return OperationResult{}, err
		}
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
	xrayStats, xrayErr := r.listXrayClientStats(ctx)
	if xrayErr != nil {
		r.log("xray.client.stats.error", slog.String("error", xrayErr.Error()))
		return stats, nil
	}
	stats = append(stats, xrayStats...)
	return stats, nil
}

func (r *AmneziaDockerRuntime) listXrayClientStats(ctx context.Context) ([]PeerStat, error) {
	if strings.TrimSpace(r.cfg.XrayContainer) == "" || strings.TrimSpace(r.cfg.XrayConfigPath) == "" {
		return nil, nil
	}
	if err := r.detectXrayRestart(ctx); err != nil {
		r.log("xray.runtime.restart.detect.error", slog.String("error", err.Error()))
	}
	out, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.XrayContainer, "cat", r.cfg.XrayConfigPath}, Timeout: r.commandTimeout()})
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("read xray config failed with exit_code=%d", out.ExitCode)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.Stdout), &cfg); err != nil {
		return nil, err
	}
	inbounds, ok := cfg["inbounds"].([]any)
	if !ok {
		return nil, nil
	}
	stats := make([]PeerStat, 0)
	seen := make(map[string]struct{})
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		tag := strings.TrimSpace(asString(inbound["tag"]))
		if !strings.EqualFold(strings.TrimSpace(asString(inbound["protocol"])), "vless") && !strings.EqualFold(tag, "vless-reality") {
			continue
		}
		settings, ok := inbound["settings"].(map[string]any)
		if !ok {
			continue
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}
		for _, rawClient := range clients {
			client, ok := rawClient.(map[string]any)
			if !ok {
				continue
			}
			id := strings.TrimSpace(asString(client["id"]))
			if id == "" {
				continue
			}
			id = strings.ToLower(id)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			accessID := ""
			if peer, ok := r.peersByKey[id]; ok {
				accessID = peer.DeviceAccessID
			}
			stats = append(stats, PeerStat{
				DeviceAccessID: accessID,
				Protocol:       "xray",
				PeerPublicKey:  id,
				AllowedIP:      "",
			})
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

var canonicalUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (r *AmneziaDockerRuntime) applyXrayClient(ctx context.Context, req PeerOperationRequest) error {
	r.xrayMu.Lock()
	defer r.xrayMu.Unlock()

	uuid, err := normalizeXrayUUID(req.PeerPublicKey)
	if err != nil {
		r.log("xray.api.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if strings.TrimSpace(r.cfg.XrayContainer) == "" {
		return errors.New("xray container is empty")
	}
	configPath := strings.TrimSpace(r.cfg.XrayConfigPath)
	if configPath == "" {
		return errors.New("xray config path is empty")
	}
	accessEmail := strings.TrimSpace(req.DeviceAccessID)
	if accessEmail == "" {
		accessEmail = "access-" + uuid
	}
	if err := r.detectXrayRestart(ctx); err != nil {
		r.log("xray.runtime.restart.detect.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
	}
	inboundTag := r.xrayInboundTag()
	flow := r.xrayFlow()
	r.log("xray.api.add.start",
		slog.String("operation_id", req.OperationID),
		slog.String("access_id", req.DeviceAccessID),
		slog.String("device_access_id", req.DeviceAccessID),
		slog.String("uuid", uuid),
		slog.String("inbound_tag", inboundTag),
		slog.String("container", r.cfg.XrayContainer),
		slog.String("config_path", configPath),
	)
	if _, err := r.ensureXrayBootstrapUser(ctx, req.OperationID, uuid, true); err != nil {
		r.log("xray.api.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if err := r.ensureXrayAPIReady(ctx, req.OperationID); err != nil {
		r.log("xray.api.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if current, ok := r.xrayRuntimeUsers[uuid]; ok && strings.EqualFold(current, accessEmail) {
		r.mu.Lock()
		r.peersByKey[uuid] = PeerState{DeviceAccessID: req.DeviceAccessID, Protocol: "xray", PeerPublicKey: uuid, UpdatedAt: time.Now().UTC()}
		r.mu.Unlock()
		r.log("xray.api.add.success", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.Bool("idempotent", true))
		return nil
	}
	cmdArgs := []string{"exec", r.cfg.XrayContainer, "xray", "api", "adu", "--server=" + r.xrayAPIAddress(), "--inboundTag=" + inboundTag, "--email=" + accessEmail, "--uuid=" + uuid, "--flow=" + flow}
	r.log("xray.api.add.command", slog.String("operation_id", req.OperationID), slog.String("command", strings.Join(append([]string{r.cfg.DockerBinaryPath}, cmdArgs...), " ")))
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: cmdArgs, Timeout: r.commandTimeout()})
	if err != nil {
		r.log("xray.api.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if res.ExitCode != 0 && !xrayCommandIsIdempotent(res.Stdout, res.Stderr, "add") {
		err = fmt.Errorf("xray api adu failed with exit_code=%d stderr=%s stdout=%s", res.ExitCode, strings.TrimSpace(res.Stderr), strings.TrimSpace(res.Stdout))
		r.log("xray.api.add.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	r.xrayRuntimeUsers[uuid] = accessEmail
	r.mu.Lock()
	r.peersByKey[uuid] = PeerState{DeviceAccessID: req.DeviceAccessID, Protocol: "xray", PeerPublicKey: uuid, UpdatedAt: time.Now().UTC()}
	r.mu.Unlock()
	r.log("xray.api.add.success", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.Bool("idempotent", false))
	return nil
}

func (r *AmneziaDockerRuntime) revokeXrayClient(ctx context.Context, req PeerOperationRequest) error {
	r.xrayMu.Lock()
	defer r.xrayMu.Unlock()

	uuid, err := normalizeXrayUUID(req.PeerPublicKey)
	if err != nil {
		r.log("xray.api.remove.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if strings.TrimSpace(r.cfg.XrayContainer) == "" {
		return errors.New("xray container is empty")
	}
	configPath := strings.TrimSpace(r.cfg.XrayConfigPath)
	if configPath == "" {
		return errors.New("xray config path is empty")
	}
	if err := r.detectXrayRestart(ctx); err != nil {
		r.log("xray.runtime.restart.detect.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
	}
	inboundTag := r.xrayInboundTag()
	accessEmail := strings.TrimSpace(req.DeviceAccessID)
	if accessEmail == "" {
		accessEmail = "access-" + uuid
	}
	r.log("xray.api.remove.start", slog.String("operation_id", req.OperationID), slog.String("access_id", req.DeviceAccessID), slog.String("device_access_id", req.DeviceAccessID), slog.String("uuid", uuid), slog.String("inbound_tag", inboundTag), slog.String("container", r.cfg.XrayContainer), slog.String("config_path", configPath))
	if _, err := r.ensureXrayBootstrapUser(ctx, req.OperationID, uuid, false); err != nil {
		r.log("xray.api.remove.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if err := r.ensureXrayAPIReady(ctx, req.OperationID); err != nil {
		r.log("xray.api.remove.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	cmdArgs := []string{"exec", r.cfg.XrayContainer, "xray", "api", "rmu", "--server=" + r.xrayAPIAddress(), "--inboundTag=" + inboundTag, "--email=" + accessEmail}
	r.log("xray.api.remove.command", slog.String("operation_id", req.OperationID), slog.String("command", strings.Join(append([]string{r.cfg.DockerBinaryPath}, cmdArgs...), " ")))
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: cmdArgs, Timeout: r.commandTimeout()})
	if err != nil {
		r.log("xray.api.remove.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	if res.ExitCode != 0 && !xrayCommandIsIdempotent(res.Stdout, res.Stderr, "remove") {
		err = fmt.Errorf("xray api rmu failed with exit_code=%d stderr=%s stdout=%s", res.ExitCode, strings.TrimSpace(res.Stderr), strings.TrimSpace(res.Stdout))
		r.log("xray.api.remove.error", slog.String("operation_id", req.OperationID), slog.String("error", err.Error()))
		return err
	}
	r.mu.Lock()
	delete(r.peersByKey, uuid)
	r.mu.Unlock()
	delete(r.xrayRuntimeUsers, uuid)
	r.log("xray.api.remove.success", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.Bool("idempotent", false))
	return nil
}

func normalizeXrayUUID(raw string) (string, error) {
	uuid := strings.TrimSpace(raw)
	if uuid == "" {
		return "", errors.New("xray client uuid is empty")
	}
	if !canonicalUUIDPattern.MatchString(uuid) {
		return "", fmt.Errorf("xray client id must be a canonical UUID, got %q", raw)
	}
	return strings.ToLower(uuid), nil
}

func (r *AmneziaDockerRuntime) xrayAPIAddress() string {
	if v := strings.TrimSpace(r.cfg.XrayAPIAddress); v != "" {
		return v
	}
	return "127.0.0.1:10085"
}

func (r *AmneziaDockerRuntime) xrayInboundTag() string {
	if v := strings.TrimSpace(r.cfg.XrayAPIInboundTag); v != "" {
		return v
	}
	return "vless-reality"
}

func (r *AmneziaDockerRuntime) xrayFlow() string {
	if v := strings.TrimSpace(r.cfg.XrayClientFlow); v != "" {
		return v
	}
	return "xtls-rprx-vision"
}

func (r *AmneziaDockerRuntime) ensureXrayBootstrapUser(ctx context.Context, operationID, uuid string, present bool) (bool, error) {
	out, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.XrayContainer, "cat", r.cfg.XrayConfigPath}, Timeout: r.commandTimeout()})
	if err != nil {
		return false, err
	}
	if out.ExitCode != 0 {
		return false, fmt.Errorf("read xray config failed with exit_code=%d", out.ExitCode)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.Stdout), &cfg); err != nil {
		return false, err
	}
	inbounds, ok := cfg["inbounds"].([]any)
	if !ok {
		return false, errors.New("xray config inbounds is missing or invalid")
	}
	updated := false
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]any)
		if !ok {
			continue
		}
		tag := strings.TrimSpace(asString(inbound["tag"]))
		if !strings.EqualFold(strings.TrimSpace(asString(inbound["protocol"])), "vless") && !strings.EqualFold(tag, r.xrayInboundTag()) && !strings.EqualFold(tag, "vless-reality") {
			continue
		}
		settings, ok := inbound["settings"].(map[string]any)
		if !ok {
			settings = map[string]any{}
			inbound["settings"] = settings
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			clients = []any{}
		}
		next := make([]any, 0, len(clients)+1)
		exists := false
		for _, rawClient := range clients {
			client, ok := rawClient.(map[string]any)
			if !ok {
				next = append(next, rawClient)
				continue
			}
			if strings.EqualFold(strings.TrimSpace(asString(client["id"])), uuid) {
				exists = true
				if present {
					prevID := strings.TrimSpace(asString(client["id"]))
					prevFlow := strings.TrimSpace(asString(client["flow"]))
					client["id"] = uuid
					client["flow"] = r.xrayFlow()
					if !strings.EqualFold(prevID, uuid) || !strings.EqualFold(prevFlow, r.xrayFlow()) {
						updated = true
					}
					next = append(next, client)
				} else {
					updated = true
				}
				continue
			}
			next = append(next, rawClient)
		}
		if present && !exists {
			next = append(next, map[string]any{"id": uuid, "flow": r.xrayFlow()})
			updated = true
		}
		settings["clients"] = next
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds
	if !updated {
		return false, nil
	}
	r.log("xray.bootstrap.config.write.start", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath))
	if err := validateXrayConfig(cfg); err != nil {
		return false, err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	tmpPath, err := r.writeLocalTemp(raw)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmpPath)
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"cp", tmpPath, r.cfg.XrayContainer + ":" + r.cfg.XrayConfigPath}, Timeout: r.commandTimeout()})
	if err != nil {
		r.log("xray.bootstrap.config.write.error", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath), slog.String("error", err.Error()))
		return false, err
	}
	if res.ExitCode != 0 {
		err = fmt.Errorf("docker cp xray config failed with exit_code=%d", res.ExitCode)
		r.log("xray.bootstrap.config.write.error", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath), slog.String("error", err.Error()))
		return false, err
	}
	r.log("xray.bootstrap.config.write.success", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath))
	return true, nil
}

func (r *AmneziaDockerRuntime) detectXrayRestart(ctx context.Context) error {
	if strings.TrimSpace(r.cfg.XrayContainer) == "" {
		return nil
	}
	args := []string{"inspect", "--type", "container", "--format", "{{.Id}}|{{.State.StartedAt}}", r.cfg.XrayContainer}
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("xray inspect failed with exit_code=%d", res.ExitCode)
	}
	marker := strings.TrimSpace(res.Stdout)
	if marker == "" {
		return nil
	}
	if r.xrayContainerMarker == "" {
		r.xrayContainerMarker = marker
		return nil
	}
	if marker != r.xrayContainerMarker {
		r.log("xray.runtime.restart.detected", slog.String("container", r.cfg.XrayContainer), slog.String("previous_marker", r.xrayContainerMarker), slog.String("marker", marker))
		r.xrayContainerMarker = marker
		r.xrayRuntimeUsers = make(map[string]string)
		r.log("xray.runtime.restore.after_restart", slog.String("container", r.cfg.XrayContainer))
	}
	return nil
}

func xrayCommandIsIdempotent(stdout, stderr, op string) bool {
	msg := strings.ToLower(strings.TrimSpace(stdout + " " + stderr))
	if op == "add" {
		return strings.Contains(msg, "already") && strings.Contains(msg, "exist")
	}
	if op == "remove" {
		return strings.Contains(msg, "not found") || (strings.Contains(msg, "not") && strings.Contains(msg, "exist"))
	}
	return false
}

func (r *AmneziaDockerRuntime) ensureXrayAPIReady(ctx context.Context, operationID string) error {
	if err := r.pingXrayAPI(ctx); err == nil {
		return nil
	}
	changed, err := r.ensureXrayManagementConfig(ctx, operationID)
	if err != nil {
		return err
	}
	if !changed {
		if pingErr := r.pingXrayAPI(ctx); pingErr != nil {
			return fmt.Errorf("xray api is unavailable on %s and management config was not changed: %w", r.xrayAPIAddress(), pingErr)
		}
		return nil
	}
	r.log("xray.runtime.api.bootstrap.restart.start", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer))
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"restart", r.cfg.XrayContainer}, Timeout: r.commandTimeout()})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("xray bootstrap restart failed with exit_code=%d", res.ExitCode)
	}
	r.log("xray.runtime.api.bootstrap.restart.success", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer))
	if err := r.pingXrayAPI(ctx); err != nil {
		return fmt.Errorf("xray api is still unavailable after bootstrap restart: %w", err)
	}
	return nil
}

func (r *AmneziaDockerRuntime) pingXrayAPI(ctx context.Context) error {
	args := []string{"exec", r.cfg.XrayContainer, "xray", "api", "lsi", "--server=" + r.xrayAPIAddress()}
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: args, Timeout: r.commandTimeout()})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("xray api ping failed with exit_code=%d stderr=%s stdout=%s", res.ExitCode, strings.TrimSpace(res.Stderr), strings.TrimSpace(res.Stdout))
	}
	return nil
}

func (r *AmneziaDockerRuntime) ensureXrayManagementConfig(ctx context.Context, operationID string) (bool, error) {
	out, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"exec", r.cfg.XrayContainer, "cat", r.cfg.XrayConfigPath}, Timeout: r.commandTimeout()})
	if err != nil {
		return false, err
	}
	if out.ExitCode != 0 {
		return false, fmt.Errorf("read xray config failed with exit_code=%d", out.ExitCode)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.Stdout), &cfg); err != nil {
		return false, err
	}
	changed := false
	if changedAPI := ensureMapField(cfg, "api", map[string]any{"tag": "api", "services": []any{"HandlerService", "StatsService"}}); changedAPI {
		changed = true
	}
	if changedStats := ensureMapField(cfg, "stats", map[string]any{}); changedStats {
		changed = true
	}
	if changedPolicy := ensureMapField(cfg, "policy", map[string]any{
		"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}},
		"system": map[string]any{
			"statsInboundUplink":    true,
			"statsInboundDownlink":  true,
			"statsOutboundUplink":   true,
			"statsOutboundDownlink": true,
		},
	}); changedPolicy {
		changed = true
	}
	inbounds, _ := cfg["inbounds"].([]any)
	if !hasTagProtocol(inbounds, "api", "dokodemo-door") {
		listen := "127.0.0.1"
		port := 10085
		if host, p, parseErr := net.SplitHostPort(r.xrayAPIAddress()); parseErr == nil {
			if strings.TrimSpace(host) != "" && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
				listen = host
			}
			port = mustParsePortOrDefault(p, 10085)
		}
		inbounds = append([]any{map[string]any{
			"tag":      "api",
			"listen":   listen,
			"port":     port,
			"protocol": "dokodemo-door",
			"settings": map[string]any{"address": "127.0.0.1"},
		}}, inbounds...)
		cfg["inbounds"] = inbounds
		changed = true
	}
	outbounds, _ := cfg["outbounds"].([]any)
	if !hasTag(outbounds, "api") {
		outbounds = append([]any{map[string]any{"tag": "api", "protocol": "freedom"}}, outbounds...)
		cfg["outbounds"] = outbounds
		changed = true
	}
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
	}
	rules, _ := routing["rules"].([]any)
	if !hasAPIRoutingRule(rules) {
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []any{"api"},
			"outboundTag": "api",
		})
		routing["rules"] = rules
		cfg["routing"] = routing
		changed = true
	}
	if !changed {
		return false, nil
	}
	r.log("xray.bootstrap.config.write.start", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath))
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	tmpPath, err := r.writeLocalTemp(raw)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmpPath)
	res, err := r.exec.Run(ctx, shell.ExecRequest{Bin: r.cfg.DockerBinaryPath, Args: []string{"cp", tmpPath, r.cfg.XrayContainer + ":" + r.cfg.XrayConfigPath}, Timeout: r.commandTimeout()})
	if err != nil {
		return false, err
	}
	if res.ExitCode != 0 {
		return false, fmt.Errorf("docker cp xray config failed with exit_code=%d", res.ExitCode)
	}
	r.log("xray.bootstrap.config.write.success", slog.String("operation_id", operationID), slog.String("container", r.cfg.XrayContainer), slog.String("path", r.cfg.XrayConfigPath))
	return true, nil
}

func ensureMapField(target map[string]any, key string, value map[string]any) bool {
	if existing, ok := target[key].(map[string]any); ok && len(existing) > 0 {
		return false
	}
	target[key] = value
	return true
}

func hasTag(items []any, tag string) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(asString(item["tag"])), tag) {
			return true
		}
	}
	return false
}

func hasTagProtocol(items []any, tag, protocol string) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(asString(item["tag"])), tag) &&
			strings.EqualFold(strings.TrimSpace(asString(item["protocol"])), protocol) {
			return true
		}
	}
	return false
}

func hasAPIRoutingRule(rules []any) bool {
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(asString(rule["outboundTag"])), "api") {
			continue
		}
		inboundTags, ok := rule["inboundTag"].([]any)
		if !ok {
			continue
		}
		for _, rawTag := range inboundTags {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(rawTag)), "api") {
				return true
			}
		}
	}
	return false
}

func mustParsePortOrDefault(raw string, fallback int) int {
	p, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || p <= 0 {
		return fallback
	}
	return p
}

func detectXrayClientFlow(clients []any) string {
	for _, rawClient := range clients {
		client, ok := rawClient.(map[string]any)
		if !ok {
			continue
		}
		flow := strings.TrimSpace(asString(client["flow"]))
		if flow != "" {
			return flow
		}
	}
	return "xtls-rprx-vision"
}

func validateXrayConfig(cfg map[string]any) error {
	inbounds, ok := cfg["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		return errors.New("xray config validation failed: inbounds must be a non-empty array")
	}
	for i, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		tag := strings.TrimSpace(asString(inbound["tag"]))
		if !strings.EqualFold(strings.TrimSpace(asString(inbound["protocol"])), "vless") && !strings.EqualFold(tag, "vless-reality") {
			continue
		}
		if _, ok := inbound["port"]; !ok {
			return fmt.Errorf("xray config validation failed: inbounds[%d].port is required", i)
		}
		settings, ok := inbound["settings"].(map[string]any)
		if !ok {
			return fmt.Errorf("xray config validation failed: inbounds[%d].settings is required", i)
		}
		if !strings.EqualFold(strings.TrimSpace(asString(settings["decryption"])), "none") {
			return fmt.Errorf("xray config validation failed: inbounds[%d].settings.decryption must be none", i)
		}
		if _, ok := settings["clients"].([]any); !ok {
			return fmt.Errorf("xray config validation failed: inbounds[%d].settings.clients must be an array", i)
		}
		streamSettings, ok := inbound["streamSettings"].(map[string]any)
		if !ok {
			return fmt.Errorf("xray config validation failed: inbounds[%d].streamSettings is required", i)
		}
		if !strings.EqualFold(strings.TrimSpace(asString(streamSettings["security"])), "reality") {
			return fmt.Errorf("xray config validation failed: inbounds[%d].streamSettings.security must be reality", i)
		}
		if _, ok := streamSettings["realitySettings"].(map[string]any); !ok {
			return fmt.Errorf("xray config validation failed: inbounds[%d].streamSettings.realitySettings is required", i)
		}
		return nil
	}
	return errors.New("xray config validation failed: vless inbound not found")
}

func asString(v any) string {
	s, _ := v.(string)
	return s
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
