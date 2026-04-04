package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
)

type testRuntime struct {
	stats       []runtime.PeerStat
	revokeCalls int
	applyCalls  []runtime.PeerOperationRequest
}

func (t *testRuntime) ApplyPeer(ctx context.Context, req runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	t.applyCalls = append(t.applyCalls, req)
	return runtime.OperationResult{OperationID: req.OperationID, Applied: true}, nil
}
func (t *testRuntime) RevokePeer(ctx context.Context, req runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	t.revokeCalls++
	return runtime.OperationResult{OperationID: req.OperationID, Applied: true}, nil
}
func (t *testRuntime) ListPeerStats(ctx context.Context) ([]runtime.PeerStat, error) {
	return t.stats, nil
}
func (t *testRuntime) Health(ctx context.Context) error { return nil }

func TestReconcileRuntime_RevokesUnknownAccessWithID(t *testing.T) {
	rt := &testRuntime{stats: []runtime.PeerStat{{DeviceAccessID: "stale-access", Protocol: "wireguard", AllowedIP: "10.0.0.2/32"}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	results := reconcileRuntime(context.Background(), logger, rt, nil)
	if rt.revokeCalls != 1 {
		t.Fatalf("expected one revoke call, got %d", rt.revokeCalls)
	}
	if len(results) == 0 || results[0]["status"] != "ok" {
		t.Fatalf("expected successful revoke result, got %#v", results)
	}
}

func TestReconcileRuntime_RevokesOrphanRuntimePeerWithoutAccessID(t *testing.T) {
	rt := &testRuntime{stats: []runtime.PeerStat{{Protocol: "wireguard", PeerPublicKey: "pk-orphan", AllowedIP: "10.0.0.88/32"}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	results := reconcileRuntime(context.Background(), logger, rt, nil)
	if rt.revokeCalls != 1 {
		t.Fatalf("expected one orphan revoke call, got %d", rt.revokeCalls)
	}
	if len(results) == 0 || results[0]["status"] != "ok" {
		t.Fatalf("expected successful orphan revoke result, got %#v", results)
	}
}

func TestReconcileRuntime_DoesNotRevokeOrphanWhenPeerPresentInDesiredByFingerprint(t *testing.T) {
	rt := &testRuntime{stats: []runtime.PeerStat{{Protocol: "xray", PeerPublicKey: "11111111-1111-1111-1111-111111111111", AllowedIP: ""}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	desired := []struct {
		AccessID            string         `json:"access_id"`
		Protocol            string         `json:"protocol"`
		PeerPublicKey       string         `json:"peer_public_key"`
		AssignedIP          string         `json:"assigned_ip"`
		PersistentKeepalive int            `json:"persistent_keepalive"`
		EndpointParams      map[string]any `json:"endpoint_params"`
	}{
		{AccessID: "access-x", Protocol: "xray", PeerPublicKey: "11111111-1111-1111-1111-111111111111", AssignedIP: ""},
	}

	_ = reconcileRuntime(context.Background(), logger, rt, desired)
	if rt.revokeCalls != 0 {
		t.Fatalf("did not expect revoke for desired xray orphan, got %d", rt.revokeCalls)
	}
}

func TestReconcileRuntime_PassesEndpointParamsToApply(t *testing.T) {
	rt := &testRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	desired := []struct {
		AccessID            string         `json:"access_id"`
		Protocol            string         `json:"protocol"`
		PeerPublicKey       string         `json:"peer_public_key"`
		AssignedIP          string         `json:"assigned_ip"`
		PersistentKeepalive int            `json:"persistent_keepalive"`
		EndpointParams      map[string]any `json:"endpoint_params"`
	}{
		{
			AccessID:            "a-wg-1",
			Protocol:            "wireguard",
			PeerPublicKey:       "peer1",
			AssignedIP:          "10.8.1.9/32",
			PersistentKeepalive: 25,
			EndpointParams:      map[string]any{"preshared_key": "psk-value"},
		},
	}
	_ = reconcileRuntime(context.Background(), logger, rt, desired)
	if len(rt.applyCalls) != 1 {
		t.Fatalf("expected one apply call, got %d", len(rt.applyCalls))
	}
	if got := rt.applyCalls[0].EndpointMeta["preshared_key"]; got != "psk-value" {
		t.Fatalf("expected endpoint meta preshared_key to be passed, got %q", got)
	}
}
