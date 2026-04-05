package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type fakeExecutor struct {
	calls []shell.ExecRequest
	res   shell.ExecResult
	err   error
}

func (f *fakeExecutor) Run(ctx context.Context, req shell.ExecRequest) (shell.ExecResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return shell.ExecResult{}, f.err
	}
	return f.res, nil
}

func TestShellRuntime_HealthOK(t *testing.T) {
	tmp := t.TempDir()
	awg := filepath.Join(tmp, "awg")
	wg := filepath.Join(tmp, "wg")
	ip := filepath.Join(tmp, "ip")
	for _, p := range []string{awg, wg, ip} {
		if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	exec := &fakeExecutor{res: shell.ExecResult{ExitCode: 0}}
	rt := NewShellRuntime(nil, ShellRuntimeConfig{
		WorkDir:        tmp,
		AWGBinaryPath:  awg,
		WGBinaryPath:   wg,
		IPBinaryPath:   ip,
		CommandTimeout: 2 * time.Second,
	}, exec)

	if err := rt.Health(context.Background()); err != nil {
		t.Fatalf("unexpected health err: %v", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 version checks, got %d", len(exec.calls))
	}
	if exec.calls[0].Args[0] != "--version" {
		t.Fatalf("expected --version check, got %v", exec.calls[0].Args)
	}
}

func TestShellRuntime_HealthFailsWhenVersionFails(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "awg")
	if err := os.WriteFile(bin, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exec := &fakeExecutor{err: errors.New("boom")}
	rt := NewShellRuntime(nil, ShellRuntimeConfig{
		WorkDir:       tmp,
		AWGBinaryPath: bin,
		WGBinaryPath:  bin,
		IPBinaryPath:  bin,
	}, exec)

	if err := rt.Health(context.Background()); err == nil {
		t.Fatal("expected health error")
	}
}

func TestShellRuntime_ApplyRevokeNotImplemented(t *testing.T) {
	rt := NewShellRuntime(nil, ShellRuntimeConfig{}, &fakeExecutor{})
	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{OperationID: "op1", DeviceAccessID: "d1"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
	_, err = rt.RevokePeer(context.Background(), PeerOperationRequest{OperationID: "op2", DeviceAccessID: "d2"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}
