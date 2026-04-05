package runtime

import (
	"context"
	"log/slog"
	"testing"
)

func TestMockRuntime_ApplyPeerIdempotent(t *testing.T) {
	rt := NewMockRuntime(slog.Default())
	req := PeerOperationRequest{OperationID: "op-1", DeviceAccessID: "da-1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25}

	res1, err := rt.ApplyPeer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res1.Idempotent {
		t.Fatal("first call must not be idempotent")
	}

	res2, err := rt.ApplyPeer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res2.Idempotent {
		t.Fatal("second call must be idempotent")
	}
}

func TestMockRuntime_RevokePeerIdempotent(t *testing.T) {
	rt := NewMockRuntime(slog.Default())
	applyReq := PeerOperationRequest{OperationID: "op-apply", DeviceAccessID: "da-1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25}
	_, _ = rt.ApplyPeer(context.Background(), applyReq)

	revokeReq := PeerOperationRequest{OperationID: "op-revoke", DeviceAccessID: "da-1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25}
	res1, err := rt.RevokePeer(context.Background(), revokeReq)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res1.Idempotent {
		t.Fatal("first revoke should not be idempotent")
	}

	res2, err := rt.RevokePeer(context.Background(), revokeReq)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res2.Idempotent {
		t.Fatal("second revoke should be idempotent")
	}
}

func TestMockRuntime_OperationConflict(t *testing.T) {
	rt := NewMockRuntime(slog.Default())
	_, _ = rt.ApplyPeer(context.Background(), PeerOperationRequest{OperationID: "op-1", DeviceAccessID: "da-1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25})

	_, err := rt.RevokePeer(context.Background(), PeerOperationRequest{OperationID: "op-1", DeviceAccessID: "da-1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25})
	if err == nil {
		t.Fatal("expected conflict error")
	}
}
