package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
)

type amneziaFakeExecutor struct {
	calls []shell.ExecRequest
	res   []shell.ExecResult
}

func (f *amneziaFakeExecutor) Run(ctx context.Context, req shell.ExecRequest) (shell.ExecResult, error) {
	f.calls = append(f.calls, req)
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
