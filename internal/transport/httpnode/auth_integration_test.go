package httpnode

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
	"github.com/wwwcont/ryazanvpn/internal/infra/nodeclient"
)

type authRuntime struct {
	applyCalls int
	statsCalls int
}

func (a *authRuntime) ApplyPeer(context.Context, runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	a.applyCalls++
	return runtime.OperationResult{Applied: true}, nil
}

func (a *authRuntime) RevokePeer(context.Context, runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	return runtime.OperationResult{Applied: true}, nil
}

func (a *authRuntime) ListPeerStats(context.Context) ([]runtime.PeerStat, error) {
	a.statsCalls++
	return []runtime.PeerStat{{DeviceAccessID: "da-1", RXTotalBytes: 10, TXTotalBytes: 20}}, nil
}

func (a *authRuntime) Health(context.Context) error { return nil }

func TestAgentHMACIntegration_TrafficCountersAcceptedWithCorrectSecret(t *testing.T) {
	rt := &authRuntime{}
	server := httptest.NewServer(NewRouter(Options{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadinessTimeout: time.Second,
		Runtime:          rt,
		HMACSecret:       "secret",
		HMACMaxSkew:      time.Minute,
	}))
	defer server.Close()

	client := nodeclient.New(nodeclient.Config{BaseURL: server.URL, Secret: "secret", Timeout: time.Second})
	items, err := client.GetTrafficCounters(context.Background())
	if err != nil {
		t.Fatalf("GetTrafficCounters error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 traffic item, got %d", len(items))
	}
	if rt.statsCalls != 1 {
		t.Fatalf("expected traffic runtime call, got %d", rt.statsCalls)
	}
}

func TestAgentHMACIntegration_TrafficCountersRejectedWithWrongSecret(t *testing.T) {
	rt := &authRuntime{}
	server := httptest.NewServer(NewRouter(Options{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadinessTimeout: time.Second,
		Runtime:          rt,
		HMACSecret:       "secret",
		HMACMaxSkew:      time.Minute,
	}))
	defer server.Close()

	client := nodeclient.New(nodeclient.Config{BaseURL: server.URL, Secret: "wrong", Timeout: time.Second})
	_, err := client.GetTrafficCounters(context.Background())
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
	if rt.statsCalls != 0 {
		t.Fatalf("expected zero runtime calls on bad signature, got %d", rt.statsCalls)
	}
}

func TestAgentHMACIntegration_SameSignerWorksForTrafficAndPeerOperations(t *testing.T) {
	rt := &authRuntime{}
	server := httptest.NewServer(NewRouter(Options{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadinessTimeout: time.Second,
		Runtime:          rt,
		HMACSecret:       "secret",
		HMACMaxSkew:      time.Minute,
	}))
	defer server.Close()

	client := nodeclient.New(nodeclient.Config{BaseURL: server.URL, Secret: "secret", Timeout: time.Second})
	if _, err := client.GetTrafficCounters(context.Background()); err != nil {
		t.Fatalf("GetTrafficCounters error: %v", err)
	}
	if err := client.ApplyPeer(context.Background(), nodeclient.OperationRequest{
		OperationID:    "op-1",
		DeviceAccessID: "da-1",
		Protocol:       "wireguard",
		PeerPublicKey:  "pk",
		AssignedIP:     "10.0.0.2/32",
		Keepalive:      25,
	}); err != nil {
		t.Fatalf("ApplyPeer error: %v", err)
	}

	if rt.statsCalls != 1 {
		t.Fatalf("expected one stats call, got %d", rt.statsCalls)
	}
	if rt.applyCalls != 1 {
		t.Fatalf("expected one apply call, got %d", rt.applyCalls)
	}
}

// compile-time guard
var _ runtime.VPNRuntime = (*authRuntime)(nil)
