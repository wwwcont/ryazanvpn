package app

import (
	"context"
	"log/slog"
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
}

func (r *pcAccessRepo) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return nil, nil
}
func (r *pcAccessRepo) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	return nil, nil
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

type pcLogCollector struct {
	mu        sync.Mutex
	warnCount int
}

func (h *pcLogCollector) Enabled(ctx context.Context, level slog.Level) bool { return true }
func (h *pcLogCollector) Handle(ctx context.Context, rec slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if rec.Level >= slog.LevelWarn {
		h.warnCount++
	}
	return nil
}
func (h *pcLogCollector) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *pcLogCollector) WithGroup(name string) slog.Handler       { return h }

func strPtrPC(v string) *string { return &v }
