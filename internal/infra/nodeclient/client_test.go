package nodeclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ApplyPeer_SuccessAfterRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Secret: "secret", Timeout: time.Second, MaxRetries: 2})
	err := c.ApplyPeer(context.Background(), OperationRequest{OperationID: "op1", DeviceAccessID: "da1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestClient_ApplyPeer_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Secret: "secret", Timeout: time.Second, MaxRetries: 1})
	err := c.ApplyPeer(context.Background(), OperationRequest{OperationID: "op1", DeviceAccessID: "da1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ApplyPeer_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Secret: "secret", Timeout: 50 * time.Millisecond, MaxRetries: 0})
	err := c.ApplyPeer(context.Background(), OperationRequest{OperationID: "op1", DeviceAccessID: "da1", Protocol: "wireguard", PeerPublicKey: "pk", AssignedIP: "10.0.0.2/32", Keepalive: 25})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
