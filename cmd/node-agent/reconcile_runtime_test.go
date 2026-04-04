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
}

func (t *testRuntime) ApplyPeer(ctx context.Context, req runtime.PeerOperationRequest) (runtime.OperationResult, error) {
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
