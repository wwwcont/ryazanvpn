package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
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
		r.mu.Lock()
		r.peersByKey[req.PeerPublicKey] = PeerState{DeviceAccessID: req.DeviceAccessID, Protocol: "xray", PeerPublicKey: req.PeerPublicKey, AssignedIP: req.AssignedIP, Keepalive: req.Keepalive, UpdatedAt: time.Now().UTC()}
		r.mu.Unlock()
		r.log("runtime.amnezia_docker.apply_peer.xray.accepted", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID))
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
	if req.Keepalive > 0 {
		args = append(args, "persistent-keepalive", strconv.Itoa(req.Keepalive))
	}

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

	r.log("runtime.amnezia_docker.apply_peer.ok", slog.String("operation_id", req.OperationID), slog.String("device_access_id", req.DeviceAccessID), slog.String("peer_public_key", req.PeerPublicKey), slog.String("assigned_ip", assignedIP))
	return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
}

func (r *AmneziaDockerRuntime) RevokePeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error) {
	if strings.TrimSpace(req.OperationID) == "" || strings.TrimSpace(req.DeviceAccessID) == "" || strings.TrimSpace(req.PeerPublicKey) == "" {
		return OperationResult{}, errors.New("operation_id, device_access_id and peer_public_key are required")
	}
	if strings.EqualFold(strings.TrimSpace(req.Protocol), "xray") {
		r.mu.Lock()
		delete(r.peersByKey, req.PeerPublicKey)
		r.mu.Unlock()
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
