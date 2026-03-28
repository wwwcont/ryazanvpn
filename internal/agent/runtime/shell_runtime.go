package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type ShellRuntimeConfig struct {
	WorkDir         string
	AWGBinaryPath   string
	WGBinaryPath    string
	IPBinaryPath    string
	StatsBinaryPath string
	StatsArgs       []string
	CommandTimeout  time.Duration
}

type ShellRuntime struct {
	logger *slog.Logger
	cfg    ShellRuntimeConfig
	exec   shell.CommandExecutor
}

func NewShellRuntime(logger *slog.Logger, cfg ShellRuntimeConfig, executor shell.CommandExecutor) *ShellRuntime {
	return &ShellRuntime{logger: logger, cfg: cfg, exec: executor}
}

func (s *ShellRuntime) ApplyPeer(_ context.Context, req PeerOperationRequest) (OperationResult, error) {
	s.log("runtime.shell.apply_peer.requested",
		slog.String("operation_id", req.OperationID),
		slog.String("device_access_id", req.DeviceAccessID),
		slog.String("assigned_ip", req.AssignedIP),
		slog.Duration("command_timeout", s.commandTimeout()),
	)

	// TODO: Implement verified and hardened AWG/WG peer-apply flow.
	// Intentionally not executing network-affecting commands in MVP template.
	return OperationResult{}, fmt.Errorf("apply peer for shell runtime: %w", ErrNotImplemented)
}

func (s *ShellRuntime) RevokePeer(_ context.Context, req PeerOperationRequest) (OperationResult, error) {
	s.log("runtime.shell.revoke_peer.requested",
		slog.String("operation_id", req.OperationID),
		slog.String("device_access_id", req.DeviceAccessID),
		slog.Duration("command_timeout", s.commandTimeout()),
	)

	// TODO: Implement verified and hardened AWG/WG peer-revoke flow.
	// Intentionally not executing network-affecting commands in MVP template.
	return OperationResult{}, fmt.Errorf("revoke peer for shell runtime: %w", ErrNotImplemented)
}

func (s *ShellRuntime) Health(ctx context.Context) error {
	if s.exec == nil {
		return errors.New("shell executor is nil")
	}

	if strings.TrimSpace(s.cfg.WorkDir) == "" {
		return errors.New("runtime workdir is empty")
	}
	if err := os.MkdirAll(s.cfg.WorkDir, 0o750); err != nil {
		return fmt.Errorf("ensure runtime workdir: %w", err)
	}

	for _, p := range []struct {
		name string
		path string
	}{
		{name: "awg", path: s.cfg.AWGBinaryPath},
		{name: "wg", path: s.cfg.WGBinaryPath},
		{name: "ip", path: s.cfg.IPBinaryPath},
	} {
		if err := s.checkBinary(ctx, p.name, p.path); err != nil {
			return err
		}
	}

	s.log("runtime.shell.health.ok", slog.String("work_dir", s.cfg.WorkDir))
	return nil
}

func (s *ShellRuntime) ListPeerStats(ctx context.Context) ([]PeerStat, error) {
	if strings.TrimSpace(s.cfg.StatsBinaryPath) == "" {
		return nil, fmt.Errorf("list peer stats for shell runtime: %w", ErrNotImplemented)
	}
	if len(s.cfg.StatsArgs) == 0 {
		return nil, fmt.Errorf("stats args are required: %w", ErrNotImplemented)
	}
	for _, a := range s.cfg.StatsArgs {
		if !safeShellArg(a) {
			return nil, fmt.Errorf("unsafe stats arg %q", a)
		}
	}

	s.log("runtime.shell.list_peer_stats.requested", slog.String("bin", s.cfg.StatsBinaryPath), slog.Any("args", s.cfg.StatsArgs))
	res, err := s.exec.Run(ctx, shell.ExecRequest{Bin: s.cfg.StatsBinaryPath, Args: s.cfg.StatsArgs, WorkDir: s.cfg.WorkDir, Timeout: s.commandTimeout()})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("stats command failed with exit_code=%d", res.ExitCode)
	}
	stats, err := parsePeerStatsOutput(res.Stdout)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func parsePeerStatsOutput(out string) ([]PeerStat, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	stats := make([]PeerStat, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid stats line: %q", line)
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`).MatchString(parts[0]) {
			return nil, fmt.Errorf("invalid device_access_id token: %q", parts[0])
		}
		rx, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || rx < 0 {
			return nil, fmt.Errorf("invalid rx bytes: %q", parts[1])
		}
		tx, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || tx < 0 {
			return nil, fmt.Errorf("invalid tx bytes: %q", parts[2])
		}
		var hs *time.Time
		if len(parts) > 3 && parts[3] != "-" {
			ts, err := strconv.ParseInt(parts[3], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid handshake ts: %q", parts[3])
			}
			t := time.Unix(ts, 0).UTC()
			hs = &t
		}
		stats = append(stats, PeerStat{DeviceAccessID: parts[0], RXTotalBytes: rx, TXTotalBytes: tx, LastHandshakeAt: hs})
	}
	return stats, nil
}

func safeShellArg(v string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9._:/=-]+$`).MatchString(v)
}

func (s *ShellRuntime) checkBinary(ctx context.Context, name, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s binary path is empty", name)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s binary check failed: %w", name, err)
	}

	res, err := s.exec.Run(ctx, shell.ExecRequest{
		Bin:     path,
		Args:    []string{"--version"},
		WorkDir: s.cfg.WorkDir,
		Timeout: s.commandTimeout(),
	})
	if err != nil {
		return fmt.Errorf("%s --version failed: %w", name, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s --version returned non-zero exit code: %d", name, res.ExitCode)
	}
	return nil
}

func (s *ShellRuntime) commandTimeout() time.Duration {
	if s.cfg.CommandTimeout <= 0 {
		return 10 * time.Second
	}
	return s.cfg.CommandTimeout
}

func (s *ShellRuntime) log(msg string, attrs ...any) {
	if s.logger == nil {
		slog.Default().Info(msg, attrs...)
		return
	}
	s.logger.Info(msg, attrs...)
}
