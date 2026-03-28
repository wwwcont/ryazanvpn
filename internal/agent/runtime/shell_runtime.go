package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type ShellRuntimeConfig struct {
	WorkDir        string
	AWGBinaryPath  string
	WGBinaryPath   string
	IPBinaryPath   string
	CommandTimeout time.Duration
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
