package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/shared/contracts/nodeapi"
)

func TestControlPlaneClient_SendsProtocolVersionHeader(t *testing.T) {
	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get(nodeapi.HeaderProtocolVersion)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"node_status":"active","peers":[]}`))
	}))
	defer srv.Close()

	c := controlPlaneClient{baseURL: srv.URL, secret: []byte("secret"), client: &http.Client{Timeout: time.Second}, nodeID: "n1", token: "t1"}
	if _, err := c.getDesired(context.Background(), "n1", "t1"); err != nil {
		t.Fatalf("getDesired failed: %v", err)
	}
	if gotVersion != nodeapi.CurrentProtocolVersion {
		t.Fatalf("expected protocol version %s, got %s", nodeapi.CurrentProtocolVersion, gotVersion)
	}
}

func TestControlPlaneClient_PostSendsProtocolVersionHeader(t *testing.T) {
	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get(nodeapi.HeaderProtocolVersion)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := controlPlaneClient{baseURL: srv.URL, secret: []byte("secret"), client: &http.Client{Timeout: time.Second}, nodeID: "n1", token: "t1"}
	if err := c.post(context.Background(), "/nodes/heartbeat", map[string]any{"node_id": "n1"}); err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if gotVersion != nodeapi.CurrentProtocolVersion {
		t.Fatalf("expected protocol version %s, got %s", nodeapi.CurrentProtocolVersion, gotVersion)
	}
}

func TestDesiredStateResponse_JSONContract(t *testing.T) {
	body := []byte(`{"node_status":"active","peers":[{"access_id":"a1","protocol":"wireguard","peer_public_key":"pk","assigned_ip":"10.0.0.2/32","persistent_keepalive":25,"endpoint_params":{}}]}`)
	var out desiredStateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(out.Peers) != 1 || out.Peers[0].AccessID != "a1" {
		t.Fatalf("unexpected contract decode: %+v", out)
	}
}
