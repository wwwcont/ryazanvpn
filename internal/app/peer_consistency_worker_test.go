package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

func TestPeerConsistencyWorker_LogsMismatches(t *testing.T) {
	repo := &pcNodeRepo{nodes: []*node.Node{{ID: "n1", AgentBaseURL: "http://n1"}}}
	accessRepo := &pcAccessRepo{byNode: map[string][]*access.DeviceAccess{
		"n1": {{ID: "a-db-only", AssignedIP: strPtrPC("10.0.0.2/32")}},
	}}
	logs := &pcLogCollector{}
	worker := PeerConsistencyWorker{
		Logger:        slog.New(logs),
		Nodes:         repo,
		Accesses:      accessRepo,
		ClientFactory: &pcFactory{items: []NodeTrafficCounter{{DeviceAccessID: "a-runtime-only", AllowedIP: "10.0.0.3/32"}}},
	}

	worker.check(context.Background())
	if logs.warnCount < 2 {
		t.Fatalf("expected at least 2 warning logs for mismatches, got %d", logs.warnCount)
	}
	if !logs.hasMessage("peer_consistency.control_plane_missing") {
		t.Fatal("expected peer_consistency.control_plane_missing log")
	}
	if !logs.hasMessage("peer_consistency.runtime_missing") {
		t.Fatal("expected peer_consistency.runtime_missing log")
	}
}

func TestPeerConsistencyWorker_IgnoresXrayProtocol(t *testing.T) {
	repo := &pcNodeRepo{nodes: []*node.Node{{ID: "n1", AgentBaseURL: "http://n1"}}}
	accessRepo := &pcAccessRepo{byNode: map[string][]*access.DeviceAccess{
		"n1": {{ID: "a-xray", Protocol: "xray", AssignedIP: strPtrPC("10.0.0.9/32")}},
	}}
	logs := &pcLogCollector{}
	worker := PeerConsistencyWorker{
		Logger:   slog.New(logs),
		Nodes:    repo,
		Accesses: accessRepo,
		ClientFactory: &pcFactory{items: []NodeTrafficCounter{
			{DeviceAccessID: "a-xray", Protocol: "xray", AllowedIP: "10.0.0.9/32"},
		}},
	}

	worker.check(context.Background())
	if logs.warnCount != 0 {
		t.Fatalf("expected no mismatch logs for xray protocol, got %d", logs.warnCount)
	}
}

func TestPeerConsistencyWorker_LogsStaleNodeReference(t *testing.T) {
	repo := &pcNodeRepo{nodes: []*node.Node{{ID: "n1", AgentBaseURL: "http://n1"}}}
	accessRepo := &pcAccessRepo{
		byNode: map[string][]*access.DeviceAccess{"n1": {}},
		byID:   map[string]*access.DeviceAccess{"a-other": {ID: "a-other", VPNNodeID: "n-stale", Protocol: "wireguard"}},
	}
	logs := &pcLogCollector{}
	worker := PeerConsistencyWorker{
		Logger:        slog.New(logs),
		Nodes:         repo,
		Accesses:      accessRepo,
		ClientFactory: &pcFactory{items: []NodeTrafficCounter{{DeviceAccessID: "a-other", Protocol: "wireguard", AllowedIP: "10.0.0.3/32"}}},
	}
	worker.check(context.Background())
	if !logs.hasMessage("peer_consistency.stale_node_reference") {
		t.Fatal("expected stale node reference log")
	}
}

func TestPeerConsistencyWorker_ReconcileMissingPeerPath(t *testing.T) {
	repo := &pcNodeRepo{nodes: []*node.Node{{ID: "n1", AgentBaseURL: "http://n1"}}}
	accessRepo := &pcAccessRepo{byNode: map[string][]*access.DeviceAccess{"n1": {}}}
	logs := &pcLogCollector{}
	reconciler := &pcReconciler{ok: true}
	worker := PeerConsistencyWorker{
		Logger:        slog.New(logs),
		Nodes:         repo,
		Accesses:      accessRepo,
		ClientFactory: &pcFactory{items: []NodeTrafficCounter{{DeviceAccessID: "missing-access", AllowedIP: "10.0.0.30/32"}}},
		Reconciler:    reconciler,
	}

	worker.check(context.Background())
	if reconciler.calls != 1 {
		t.Fatalf("expected reconciler to be called once, got %d", reconciler.calls)
	}
	if !logs.hasMessage("peer_consistency.control_plane_missing") {
		t.Fatal("expected summary control_plane_missing log")
	}
}

type pcNodeRepo struct{ nodes []*node.Node }

func (r *pcNodeRepo) ListActive(ctx context.Context) ([]*node.Node, error)       { return r.nodes, nil }
func (r *pcNodeRepo) GetByID(ctx context.Context, id string) (*node.Node, error) { return nil, nil }
func (r *pcNodeRepo) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	return nil
}
func (r *pcNodeRepo) UpdateLoad(ctx context.Context, id string, currentLoad int) error { return nil }

type pcAccessRepo struct {
	byNode map[string][]*access.DeviceAccess
	byID   map[string]*access.DeviceAccess
}

func (r *pcAccessRepo) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return nil, nil
}
func (r *pcAccessRepo) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	if item, ok := r.byID[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}
func (r *pcAccessRepo) Activate(ctx context.Context, id string, grantedAt time.Time) error {
	return nil
}
func (r *pcAccessRepo) Revoke(ctx context.Context, id string, revokedAt time.Time) error { return nil }
func (r *pcAccessRepo) MarkError(ctx context.Context, id string, failedAt time.Time, message string) error {
	return nil
}
func (r *pcAccessRepo) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	return nil
}
func (r *pcAccessRepo) ClearConfigBlobEncrypted(ctx context.Context, id string) error { return nil }
func (r *pcAccessRepo) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}
func (r *pcAccessRepo) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	return nil, access.ErrNotFound
}
func (r *pcAccessRepo) GetActiveByNodeAndPublicKey(ctx context.Context, nodeID string, publicKey string) (*access.DeviceAccess, error) {
	return nil, access.ErrNotFound
}
func (r *pcAccessRepo) ListActiveByNodeID(ctx context.Context, nodeID string) ([]*access.DeviceAccess, error) {
	return r.byNode[nodeID], nil
}

type pcFactory struct{ items []NodeTrafficCounter }

func (f *pcFactory) ForNode(agentBaseURL string) NodeTrafficClient {
	return pcTrafficClient{items: f.items}
}

type pcTrafficClient struct{ items []NodeTrafficCounter }

func (c pcTrafficClient) GetTrafficCounters(ctx context.Context) ([]NodeTrafficCounter, error) {
	return c.items, nil
}

type pcReconciler struct {
	ok    bool
	calls int
}

func (r *pcReconciler) ReconcileMissingPeer(ctx context.Context, nodeID string, counter NodeTrafficCounter) (bool, error) {
	r.calls++
	return r.ok, nil
}

type pcLogCollector struct {
	mu        sync.Mutex
	warnCount int
	messages  []string
}

func (h *pcLogCollector) Enabled(ctx context.Context, level slog.Level) bool { return true }
func (h *pcLogCollector) Handle(ctx context.Context, rec slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if rec.Level >= slog.LevelWarn {
		h.warnCount++
		h.messages = append(h.messages, rec.Message)
	}
	return nil
}
func (h *pcLogCollector) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *pcLogCollector) WithGroup(name string) slog.Handler       { return h }
func (h *pcLogCollector) hasMessage(part string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, msg := range h.messages {
		if strings.Contains(msg, part) {
			return true
		}
	}
	return false
}

func strPtrPC(v string) *string { return &v }
