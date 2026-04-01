package runtime

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type amneziaFakeExecutor struct {
	calls []shell.ExecRequest
	res   []shell.ExecResult
	onRun func(shell.ExecRequest)
}

func (f *amneziaFakeExecutor) Run(ctx context.Context, req shell.ExecRequest) (shell.ExecResult, error) {
	f.calls = append(f.calls, req)
	if f.onRun != nil {
		f.onRun(req)
	}
	if len(f.res) == 0 {
		return shell.ExecResult{ExitCode: 0}, nil
	}
	out := f.res[0]
	f.res = f.res[1:]
	return out, nil
}

func TestParseAWGShowAllDump(t *testing.T) {
	input := strings.Join([]string{
		"awg0\tpriv\tpub\t51820\toff",
		"peerPub\tpeerPSK\t198.51.100.2:51820\t10.0.0.5/32\t1711600000\t100\t200\t25",
	}, "\n")

	stats, err := ParseAWGShowAllDump(input)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 peer stat, got %d", len(stats))
	}
	if stats[0].PeerPublicKey != "peerPub" || stats[0].AllowedIP != "10.0.0.5" {
		t.Fatalf("unexpected parsed data: %+v", stats[0])
	}
	if stats[0].RXTotalBytes != 100 || stats[0].TXTotalBytes != 200 || stats[0].LatestHandshakeUnixTime != 1711600000 {
		t.Fatalf("unexpected counters: %+v", stats[0])
	}
}

func TestAmneziaDockerRuntime_ApplyPeerBuildsCommand(t *testing.T) {
	exec := &amneziaFakeExecutor{}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{DockerBinaryPath: "/usr/bin/docker", ContainerName: "amnezia-awg2", InterfaceName: "awg0", CommandTimeout: time.Second}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-1",
		DeviceAccessID: "da-1",
		PeerPublicKey:  "pub1",
		AssignedIP:     "10.0.0.5",
		Keepalive:      25,
		EndpointMeta:   map[string]string{"preshared_key": "abcDEF123+/="},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) < 2 {
		t.Fatalf("expected at least 2 docker calls, got %d", len(exec.calls))
	}
	last := exec.calls[len(exec.calls)-2]
	joined := strings.Join(last.Args, " ")
	if !strings.Contains(joined, "exec amnezia-awg2 awg set awg0 peer pub1") || !strings.Contains(joined, "allowed-ips 10.0.0.5/32") {
		t.Fatalf("unexpected apply args: %v", last.Args)
	}
	if strings.Contains(joined, "persistent-keepalive") {
		t.Fatalf("server-side apply must not include persistent-keepalive: %v", last.Args)
	}
}

func TestAmneziaDockerRuntime_RevokePeerBuildsCommand(t *testing.T) {
	exec := &amneziaFakeExecutor{}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{DockerBinaryPath: "/usr/bin/docker", ContainerName: "amnezia-awg2", InterfaceName: "awg0", CommandTimeout: time.Second}, exec)

	_, err := rt.RevokePeer(context.Background(), PeerOperationRequest{OperationID: "op-r", DeviceAccessID: "da-1", PeerPublicKey: "pub1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(exec.calls))
	}
	want := []string{"exec", "amnezia-awg2", "awg", "set", "awg0", "peer", "pub1", "remove"}
	for i := range want {
		if exec.calls[0].Args[i] != want[i] {
			t.Fatalf("unexpected revoke args: %v", exec.calls[0].Args)
		}
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayAddsClientAndRestarts(t *testing.T) {
	workDir := t.TempDir()
	config := `{"inbounds":[{"port":443,"listen":"0.0.0.0","protocol":"vless","settings":{"clients":[],"decryption":"none"},"streamSettings":{"security":"reality","realitySettings":{"dest":"google.com:443","serverNames":["google.com"],"privateKey":"test","shortIds":["0123456789abcdef"]}}}]}`
	var stagedConfig string
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: config},
			{ExitCode: 0},
			{ExitCode: 0},
		},
		onRun: func(req shell.ExecRequest) {
			if len(req.Args) >= 3 && req.Args[0] == "cp" {
				raw, err := os.ReadFile(req.Args[1])
				if err == nil {
					stagedConfig = string(raw)
				}
			}
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		ContainerName:    "amnezia-awg2",
		InterfaceName:    "awg0",
		XrayContainer:    "ryazanvpn-xray",
		XrayConfigPath:   "/etc/xray/config.json",
		WorkDir:          workDir,
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-x",
		DeviceAccessID: "da-x",
		Protocol:       "xray",
		PeerPublicKey:  "11111111-1111-1111-1111-111111111111",
		AssignedIP:     "10.0.0.5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 docker calls, got %d", len(exec.calls))
	}
	if got := strings.Join(exec.calls[0].Args, " "); !strings.Contains(got, "exec ryazanvpn-xray cat /etc/xray/config.json") {
		t.Fatalf("unexpected read args: %v", exec.calls[0].Args)
	}
	if got := strings.Join(exec.calls[2].Args, " "); !strings.Contains(got, "restart ryazanvpn-xray") {
		t.Fatalf("unexpected restart args: %v", exec.calls[2].Args)
	}
	cpArgs := exec.calls[1].Args
	if len(cpArgs) < 3 || cpArgs[0] != "cp" || cpArgs[2] != "ryazanvpn-xray:/etc/xray/config.json" {
		t.Fatalf("unexpected cp args: %v", cpArgs)
	}
	if stagedConfig == "" {
		t.Fatal("expected staged config content to be captured")
	}
	text := stagedConfig
	if !strings.Contains(text, `"port": 443`) {
		t.Fatalf("expected inbound port to be preserved, got: %s", text)
	}
	if !strings.Contains(text, `"realitySettings"`) {
		t.Fatalf("expected reality settings to be preserved, got: %s", text)
	}
	if !strings.Contains(text, `"id": "11111111-1111-1111-1111-111111111111"`) {
		t.Fatalf("expected xray client id to be added, got: %s", text)
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayRejectsInvalidUUIDBeforeWrite(t *testing.T) {
	exec := &amneziaFakeExecutor{}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "ryazanvpn-xray",
		XrayConfigPath:   "/etc/xray/config.json",
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-x",
		DeviceAccessID: "da-x",
		Protocol:       "xray",
		PeerPublicKey:  "fbsQWD4wqojnTsOQiR6b4A7eL9Ci/rRVg3xszJePHyI=",
		AssignedIP:     "10.0.0.5",
	})
	if err == nil {
		t.Fatal("expected invalid UUID error")
	}
	if !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected no docker calls for invalid UUID, got %d", len(exec.calls))
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayValidationFailsBeforeWrite(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: `{"inbounds":[{"protocol":"vless","settings":{"clients":[]},"streamSettings":{"security":"reality"}}]}`},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "ryazanvpn-xray",
		XrayConfigPath:   "/etc/xray/config.json",
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-x",
		DeviceAccessID: "da-x",
		Protocol:       "xray",
		PeerPublicKey:  "11111111-1111-1111-8111-111111111111",
		AssignedIP:     "10.0.0.5",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), ".port is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("expected only read call before failure, got %d", len(exec.calls))
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayMatchesInboundByTag(t *testing.T) {
	workDir := t.TempDir()
	config := `{"inbounds":[{"tag":"vless-reality","listen":"0.0.0.0","port":8443,"settings":{"clients":[],"decryption":"none"},"streamSettings":{"security":"reality","realitySettings":{"dest":"www.cloudflare.com:443"}}}],"outbounds":[{"protocol":"freedom"}]}`
	var stagedConfig string
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: config},
			{ExitCode: 0},
			{ExitCode: 0},
		},
		onRun: func(req shell.ExecRequest) {
			if len(req.Args) >= 3 && req.Args[0] == "cp" {
				raw, err := os.ReadFile(req.Args[1])
				if err == nil {
					stagedConfig = string(raw)
				}
			}
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "ryazanvpn-xray",
		XrayConfigPath:   "/etc/xray/config.json",
		WorkDir:          workDir,
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-tag",
		DeviceAccessID: "da-tag",
		Protocol:       "xray",
		PeerPublicKey:  "11111111-1111-1111-1111-111111111111",
		AssignedIP:     "10.0.0.9/32",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stagedConfig, `"id": "11111111-1111-1111-1111-111111111111"`) {
		t.Fatalf("expected xray client id to be added by tag match, got: %s", stagedConfig)
	}
}
