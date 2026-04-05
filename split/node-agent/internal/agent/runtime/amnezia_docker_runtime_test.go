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

func TestAmneziaDockerRuntime_ApplyPeer_ReusesCachedPresharedKey(t *testing.T) {
	exec := &amneziaFakeExecutor{}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{DockerBinaryPath: "/usr/bin/docker", ContainerName: "amnezia-awg2", InterfaceName: "awg0", CommandTimeout: time.Second}, exec)
	ctx := context.Background()

	_, err := rt.ApplyPeer(ctx, PeerOperationRequest{
		OperationID:    "op-cache-1",
		DeviceAccessID: "da-1",
		PeerPublicKey:  "pub-cache",
		AssignedIP:     "10.0.0.9",
		Keepalive:      25,
		EndpointMeta:   map[string]string{"preshared_key": "cachedPSK123+/="},
	})
	if err != nil {
		t.Fatalf("unexpected first apply error: %v", err)
	}
	firstSetCall := exec.calls[len(exec.calls)-2]
	if got := strings.Join(firstSetCall.Args, " "); !strings.Contains(got, "preshared-key /tmp/ryazanvpn-psk-") {
		t.Fatalf("expected first apply to include psk file, got %v", firstSetCall.Args)
	}

	_, err = rt.RevokePeer(ctx, PeerOperationRequest{
		OperationID:    "op-cache-revoke",
		DeviceAccessID: "da-1",
		PeerPublicKey:  "pub-cache",
	})
	if err != nil {
		t.Fatalf("unexpected revoke error: %v", err)
	}

	_, err = rt.ApplyPeer(ctx, PeerOperationRequest{
		OperationID:    "op-cache-2",
		DeviceAccessID: "da-1",
		PeerPublicKey:  "pub-cache",
		AssignedIP:     "10.0.0.9",
		Keepalive:      25,
		EndpointMeta:   map[string]string{},
	})
	if err != nil {
		t.Fatalf("unexpected second apply error: %v", err)
	}
	secondSetCall := exec.calls[len(exec.calls)-2]
	if got := strings.Join(secondSetCall.Args, " "); !strings.Contains(got, "preshared-key /tmp/ryazanvpn-psk-") {
		t.Fatalf("expected cached psk to be reused on second apply, got %v", secondSetCall.Args)
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

func TestAmneziaDockerRuntime_ApplyPeerXrayAddsClientViaAPIWithoutRestart(t *testing.T) {
	workDir := t.TempDir()
	config := `{"inbounds":[{"tag":"vless-reality","port":443,"listen":"0.0.0.0","protocol":"vless","settings":{"clients":[],"decryption":"none"},"streamSettings":{"security":"reality","realitySettings":{"dest":"google.com:443","serverNames":["google.com"],"privateKey":"test","shortIds":["0123456789abcdef"]}}}]}`
	var stagedPayload string
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0}, // lsi
			{ExitCode: 0, Stdout: config},
			{ExitCode: 0},
			{ExitCode: 0},
		},
		onRun: func(req shell.ExecRequest) {
			if len(req.Args) >= 3 && req.Args[0] == "cp" {
				raw, err := os.ReadFile(req.Args[1])
				if err == nil {
					stagedPayload = string(raw)
				}
			}
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath:  "/usr/bin/docker",
		ContainerName:     "amnezia-awg2",
		InterfaceName:     "awg0",
		XrayContainer:     "amnezia-xray",
		XrayConfigPath:    "/opt/amnezia/xray/server.json",
		XrayAPIAddress:    "127.0.0.1:10085",
		XrayAPIInboundTag: "vless-reality",
		XrayClientFlow:    "xtls-rprx-vision",
		WorkDir:           workDir,
		CommandTimeout:    time.Second,
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
	if len(exec.calls) < 5 {
		t.Fatalf("expected at least 5 docker calls, got %d", len(exec.calls))
	}
	if got := strings.Join(exec.calls[0].Args, " "); !strings.Contains(got, "inspect --type container --format {{.Id}}|{{.State.StartedAt}} amnezia-xray") {
		t.Fatalf("unexpected restart detection args: %v", exec.calls[0].Args)
	}
	if got := strings.Join(exec.calls[1].Args, " "); !strings.Contains(got, "exec amnezia-xray xray api lsi --server=127.0.0.1:10085") {
		t.Fatalf("unexpected api ping args: %v", exec.calls[1].Args)
	}
	if got := strings.Join(exec.calls[2].Args, " "); !strings.Contains(got, "exec amnezia-xray cat /opt/amnezia/xray/server.json") {
		t.Fatalf("unexpected read args: %v", exec.calls[2].Args)
	}
	if got := strings.Join(exec.calls[4].Args, " "); !strings.Contains(got, "exec amnezia-xray xray api adu --server=127.0.0.1:10085 /tmp/") {
		t.Fatalf("unexpected api add args: %v", exec.calls[4].Args)
	}
	cpArgs := exec.calls[3].Args
	if len(cpArgs) < 3 || cpArgs[0] != "cp" || cpArgs[2] != "amnezia-xray:/opt/amnezia/xray/server.json" {
		if len(cpArgs) < 3 || cpArgs[0] != "cp" || !strings.HasPrefix(cpArgs[2], "amnezia-xray:/tmp/") {
			t.Fatalf("unexpected cp args: %v", cpArgs)
		}
	}
	if stagedPayload == "" {
		t.Fatal("expected staged payload content to be captured")
	}
	text := stagedPayload
	if !strings.Contains(text, `"port":443`) {
		t.Fatalf("expected inbound port to be preserved, got: %s", text)
	}
	if !strings.Contains(text, `"tag":"vless-reality"`) {
		t.Fatalf("expected inbound tag to be preserved, got: %s", text)
	}
	if !strings.Contains(text, `"id":"11111111-1111-1111-1111-111111111111"`) {
		t.Fatalf("expected xray client id to be added, got: %s", text)
	}
	if !strings.Contains(text, `"email":"da-x"`) {
		t.Fatalf("expected email to be access_id, got: %s", text)
	}
	if !strings.Contains(text, `"flow":"xtls-rprx-vision"`) {
		t.Fatalf("expected xray client flow to be set, got: %s", text)
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
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 0, Stdout: `{"inbounds":[{"tag":"vless-reality","protocol":"vless","settings":{"clients":[]}}]}`},
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
	if !strings.Contains(err.Error(), "invalid or empty port") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected inspect + ping + read call before failure, got %d", len(exec.calls))
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayMatchesInboundByTag(t *testing.T) {
	workDir := t.TempDir()
	config := `{"inbounds":[{"tag":"vless-reality","listen":"0.0.0.0","port":8443,"protocol":"vless","settings":{"clients":[],"decryption":"none"},"streamSettings":{"security":"reality","realitySettings":{"dest":"www.cloudflare.com:443"}}}],"outbounds":[{"protocol":"freedom"}]}`
	var stagedPayload string
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 0, Stdout: config},
			{ExitCode: 0},
			{ExitCode: 0},
		},
		onRun: func(req shell.ExecRequest) {
			if len(req.Args) >= 3 && req.Args[0] == "cp" {
				raw, err := os.ReadFile(req.Args[1])
				if err == nil {
					stagedPayload = string(raw)
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
	if !strings.Contains(stagedPayload, `"id":"11111111-1111-1111-1111-111111111111"`) {
		t.Fatalf("expected xray client id to be added by tag match, got: %s", stagedPayload)
	}
}

func TestAmneziaDockerRuntime_ListPeerStats_IncludesXrayClientsFromConfig(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0, Stdout: "awg0\tpriv\tpub\t51820\toff\n"},
			{ExitCode: 0, Stdout: `{"inbounds":[{"protocol":"vless","settings":{"clients":[{"id":"11111111-1111-1111-1111-111111111111"}],"decryption":"none"},"streamSettings":{"security":"reality","realitySettings":{"dest":"google.com:443"}}}]}`},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		ContainerName:    "amnezia-awg2",
		InterfaceName:    "awg0",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)

	stats, err := rt.ListPeerStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected one xray stat, got %d (%+v)", len(stats), stats)
	}
	if stats[0].Protocol != "xray" || stats[0].PeerPublicKey != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected xray stat: %+v", stats[0])
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayFailsWhenSourceConfigMissing(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 1},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-missing",
		DeviceAccessID: "da-missing",
		Protocol:       "xray",
		PeerPublicKey:  "11111111-1111-1111-1111-111111111111",
		AssignedIP:     "10.0.0.9/32",
	})
	if err == nil {
		t.Fatal("expected missing source config error")
	}
	if !strings.Contains(err.Error(), "read xray config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected restart detect + ping + read call, got %d", len(exec.calls))
	}
}

func TestAmneziaDockerRuntime_RevokePeerXrayRemovesClientViaAPIWithoutRestart(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 0},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.RevokePeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-revoke-xray",
		DeviceAccessID: "da-x",
		Protocol:       "xray",
		PeerPublicKey:  "11111111-1111-1111-1111-111111111111",
		AssignedIP:     "10.0.0.5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected inspect/api-ping/api-remove docker calls, got %d", len(exec.calls))
	}
	if got := strings.Join(exec.calls[2].Args, " "); !strings.Contains(got, "exec amnezia-xray xray api rmu --server=127.0.0.1:10085 -tag=vless-reality da-x") {
		t.Fatalf("unexpected api remove args: %v", exec.calls[2].Args)
	}
	for _, call := range exec.calls {
		if strings.Contains(strings.Join(call.Args, " "), " restart ") {
			t.Fatalf("unexpected restart call: %v", call.Args)
		}
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayDoesNotRestartOnAPIUnavailable(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 1, Stderr: "failed to dial 127.0.0.1:10085"},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)

	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-no-restart",
		DeviceAccessID: "da-bootstrap",
		Protocol:       "xray",
		PeerPublicKey:  "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		AssignedIP:     "10.0.0.9",
	})
	if err == nil {
		t.Fatal("expected API unavailable error")
	}
	for _, call := range exec.calls {
		if strings.Contains(strings.Join(call.Args, " "), "restart amnezia-xray") {
			t.Fatalf("unexpected restart call: %v", call.Args)
		}
	}
}

func TestBuildXrayADUTempJSONFromInboundTemplate(t *testing.T) {
	inbound := xrayInboundTemplate{
		Tag:      "vless-reality",
		Protocol: "vless",
		Listen:   "0.0.0.0",
		Port:     8443,
		Settings: map[string]any{"decryption": "none"},
	}
	raw, err := buildXrayADUTempJSON(inbound, xrayClientForADU{
		Email: "access-1",
		ID:    "11111111-1111-1111-1111-111111111111",
		Flow:  "xtls-rprx-vision",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := string(raw)
	for _, needle := range []string{
		`"inbounds":[`,
		`"tag":"vless-reality"`,
		`"protocol":"vless"`,
		`"port":8443`,
		`"email":"access-1"`,
		`"id":"11111111-1111-1111-1111-111111111111"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %q in payload, got %s", needle, text)
		}
	}
}

func TestExtractXrayInboundTemplateByTag(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{"tag": "api", "protocol": "dokodemo-door", "port": 10085, "settings": map[string]any{}},
			map[string]any{"tag": "vless-reality", "protocol": "vless", "port": 8443, "settings": map[string]any{"decryption": "none"}},
		},
	}
	inbound, err := extractXrayInboundTemplate(cfg, "vless-reality")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inbound.Tag != "vless-reality" || inbound.Protocol != "vless" || inbound.Port != 8443 {
		t.Fatalf("unexpected inbound template: %+v", inbound)
	}
}

func TestAmneziaDockerRuntime_ApplyPeerXrayDuplicateAddIsIdempotent(t *testing.T) {
	config := `{"inbounds":[{"tag":"vless-reality","port":443,"protocol":"vless","settings":{"clients":[],"decryption":"none"}}]}`
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 0, Stdout: config},
			{ExitCode: 0},
			{ExitCode: 1, Stderr: "user already exists"},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)
	_, err := rt.ApplyPeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-dup",
		DeviceAccessID: "access-dup",
		Protocol:       "xray",
		PeerPublicKey:  "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		AssignedIP:     "10.0.0.9",
	})
	if err != nil {
		t.Fatalf("duplicate add should be idempotent, got: %v", err)
	}
}

func TestAmneziaDockerRuntime_RevokePeerXrayMissingUserIsIdempotent(t *testing.T) {
	exec := &amneziaFakeExecutor{
		res: []shell.ExecResult{
			{ExitCode: 0, Stdout: "container-id|2026-01-01T00:00:00Z"},
			{ExitCode: 0},
			{ExitCode: 1, Stderr: "not found"},
		},
	}
	rt := NewAmneziaDockerRuntime(nil, AmneziaDockerRuntimeConfig{
		DockerBinaryPath: "/usr/bin/docker",
		XrayContainer:    "amnezia-xray",
		XrayConfigPath:   "/opt/amnezia/xray/server.json",
		CommandTimeout:   time.Second,
	}, exec)
	_, err := rt.RevokePeer(context.Background(), PeerOperationRequest{
		OperationID:    "op-missing-rmu",
		DeviceAccessID: "access-missing",
		Protocol:       "xray",
		PeerPublicKey:  "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		AssignedIP:     "10.0.0.9",
	})
	if err != nil {
		t.Fatalf("missing user remove should be idempotent, got: %v", err)
	}
}
