package app

import (
	"context"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
)

func TestTrafficCollectorWorker_CollectsDeltaAndHandlesReset(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	accessRepo := &tcAccessRepo{entries: map[string]*access.DeviceAccess{"a1": {ID: "a1", DeviceID: "d1"}}}
	trafficRepo := &tcTrafficRepo{}
	factory := &tcFactory{counters: [][]NodeTrafficCounter{{{DeviceAccessID: "a1", RXTotalBytes: 100, TXTotalBytes: 50}}, {{DeviceAccessID: "a1", RXTotalBytes: 20, TXTotalBytes: 10}}}}
	w := TrafficCollectorWorker{
		Nodes:         &tcNodeRepo{nodes: []*node.Node{{ID: "n1", AgentBaseURL: "http://n1", Status: node.StatusActive}}},
		Accesses:      accessRepo,
		Traffic:       trafficRepo,
		ClientFactory: factory,
		Now:           func() time.Time { return now },
	}

	w.collect(context.Background())
	if len(trafficRepo.deltas) != 1 || trafficRepo.deltas[0].RXDelta != 100 || trafficRepo.deltas[0].TXDelta != 50 {
		t.Fatalf("unexpected first delta: %+v", trafficRepo.deltas)
	}
	if len(trafficRepo.nodeSamples) != 1 || trafficRepo.nodeSamples[0].RXDelta != 100 || trafficRepo.nodeSamples[0].TXDelta != 50 {
		t.Fatalf("unexpected first node sample: %+v", trafficRepo.nodeSamples)
	}
	if trafficRepo.cleanupCalls == 0 {
		t.Fatal("expected cleanup call after collect")
	}

	now = now.Add(1 * time.Minute)
	w.collect(context.Background())
	if len(trafficRepo.deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d", len(trafficRepo.deltas))
	}
	if trafficRepo.deltas[1].RXDelta != 20 || trafficRepo.deltas[1].TXDelta != 10 {
		t.Fatalf("expected reset-aware delta=20/10, got %+v", trafficRepo.deltas[1])
	}
	if len(trafficRepo.nodeSamples) != 2 || trafficRepo.nodeSamples[1].RXDelta != 20 || trafficRepo.nodeSamples[1].TXDelta != 10 {
		t.Fatalf("unexpected second node sample: %+v", trafficRepo.nodeSamples)
	}
}

func TestTrafficCollectorWorker_ResolveAccess_SkipsInvalidRuntimeIdentifiers(t *testing.T) {
	w := TrafficCollectorWorker{
		Accesses: &tcAccessRepo{
			entries: map[string]*access.DeviceAccess{},
		},
	}
	_, err := w.resolveAccess(context.Background(), "n1", NodeTrafficCounter{
		DeviceAccessID: "",
		AllowedIP:      "(none)",
	})
	if err == nil {
		t.Fatal("expected resolve error for empty identifiers")
	}
}

func TestTrafficCollectorWorker_ResolveAccess_ByAssignedIPWithoutCIDR(t *testing.T) {
	w := TrafficCollectorWorker{
		Accesses: &tcAccessRepo{
			entries: map[string]*access.DeviceAccess{},
			byNodeIP: map[string]*access.DeviceAccess{
				"n1|10.8.1.3": {ID: "a1", DeviceID: "d1"},
			},
		},
	}
	got, err := w.resolveAccess(context.Background(), "n1", NodeTrafficCounter{
		AllowedIP: "10.8.1.3/32",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil || got.AccessID != "a1" || got.DeviceID != "d1" {
		t.Fatalf("unexpected resolved access: %+v", got)
	}
}

func TestTrafficCollectorWorker_ResolveAccess_ByPublicKey(t *testing.T) {
	w := TrafficCollectorWorker{
		Accesses: &tcAccessRepo{
			entries: map[string]*access.DeviceAccess{},
			byNodePK: map[string]*access.DeviceAccess{
				"n1|pk-1": {ID: "a-pk", DeviceID: "d-pk"},
			},
		},
	}
	got, err := w.resolveAccess(context.Background(), "n1", NodeTrafficCounter{
		PeerPublicKey: "pk-1",
		AllowedIP:     "",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil || got.AccessID != "a-pk" {
		t.Fatalf("unexpected resolved access: %+v", got)
	}
}

type tcNodeRepo struct{ nodes []*node.Node }

func (r *tcNodeRepo) ListAll(ctx context.Context) ([]*node.Node, error)                { return r.nodes, nil }
func (r *tcNodeRepo) ListActive(ctx context.Context) ([]*node.Node, error)             { return r.nodes, nil }
func (r *tcNodeRepo) GetByID(ctx context.Context, id string) (*node.Node, error)       { return nil, nil }
func (r *tcNodeRepo) UpdateStatus(ctx context.Context, id string, status string) error { return nil }
func (r *tcNodeRepo) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	return nil
}
func (r *tcNodeRepo) UpdateLoad(ctx context.Context, id string, currentLoad int) error { return nil }

type tcAccessRepo struct {
	entries  map[string]*access.DeviceAccess
	byNodeIP map[string]*access.DeviceAccess
	byNodePK map[string]*access.DeviceAccess
}

func (r *tcAccessRepo) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return nil, nil
}
func (r *tcAccessRepo) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	return r.entries[id], nil
}
func (r *tcAccessRepo) Activate(ctx context.Context, id string, grantedAt time.Time) error {
	return nil
}
func (r *tcAccessRepo) Revoke(ctx context.Context, id string, revokedAt time.Time) error { return nil }
func (r *tcAccessRepo) MarkError(ctx context.Context, id string, failedAt time.Time, message string) error {
	return nil
}
func (r *tcAccessRepo) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	return nil
}
func (r *tcAccessRepo) ClearConfigBlobEncrypted(ctx context.Context, id string) error { return nil }
func (r *tcAccessRepo) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}
func (r *tcAccessRepo) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	if r.byNodeIP == nil {
		return nil, access.ErrNotFound
	}
	item, ok := r.byNodeIP[nodeID+"|"+assignedIP]
	if !ok {
		return nil, access.ErrNotFound
	}
	return item, nil
}
func (r *tcAccessRepo) GetActiveByNodeAndPublicKey(ctx context.Context, nodeID string, publicKey string) (*access.DeviceAccess, error) {
	if r.byNodePK == nil {
		return nil, access.ErrNotFound
	}
	item, ok := r.byNodePK[nodeID+"|"+publicKey]
	if !ok {
		return nil, access.ErrNotFound
	}
	return item, nil
}
func (r *tcAccessRepo) ListActiveByNodeID(ctx context.Context, nodeID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}

type tcTrafficRepo struct {
	snapshots    []*traffic.DeviceTrafficSnapshot
	deltas       []traffic.AddDailyUsageDeltaParams
	nodeSamples  []traffic.AddNodeThroughputSampleParams
	cleanupCalls int
}

func (r *tcTrafficRepo) CreateSnapshot(ctx context.Context, in traffic.CreateSnapshotParams) (*traffic.DeviceTrafficSnapshot, error) {
	s := &traffic.DeviceTrafficSnapshot{ID: "s", DeviceID: in.DeviceID, CapturedAt: in.CapturedAt, RXTotalBytes: in.RXTotalBytes, TXTotalBytes: in.TXTotalBytes}
	r.snapshots = append(r.snapshots, s)
	return s, nil
}
func (r *tcTrafficRepo) GetLastSnapshotByDeviceID(ctx context.Context, deviceID string, before time.Time) (*traffic.DeviceTrafficSnapshot, error) {
	for i := len(r.snapshots) - 1; i >= 0; i-- {
		if r.snapshots[i].DeviceID == deviceID && r.snapshots[i].CapturedAt.Before(before) {
			return r.snapshots[i], nil
		}
	}
	return nil, nil
}
func (r *tcTrafficRepo) AddDailyUsageDelta(ctx context.Context, in traffic.AddDailyUsageDeltaParams) error {
	r.deltas = append(r.deltas, in)
	return nil
}
func (r *tcTrafficRepo) GetDeviceTrafficTotal(ctx context.Context, deviceID string) (int64, error) {
	return 0, nil
}
func (r *tcTrafficRepo) GetDeviceTrafficLastNDays(ctx context.Context, deviceID string, days int, now time.Time) (int64, error) {
	return 0, nil
}
func (r *tcTrafficRepo) GetUserTrafficTotal(ctx context.Context, userID string) (int64, error) {
	return 0, nil
}
func (r *tcTrafficRepo) GetUserTrafficLastNDays(ctx context.Context, userID string, days int, now time.Time) (int64, error) {
	return 0, nil
}
func (r *tcTrafficRepo) AddNodeThroughputSample(ctx context.Context, in traffic.AddNodeThroughputSampleParams) error {
	r.nodeSamples = append(r.nodeSamples, in)
	return nil
}
func (r *tcTrafficRepo) CleanupNodeThroughputSamples(ctx context.Context, olderThan time.Time) error {
	r.cleanupCalls++
	return nil
}

type tcFactory struct{ counters [][]NodeTrafficCounter }

func (f *tcFactory) ForNode(agentBaseURL string) NodeTrafficClient {
	if len(f.counters) == 0 {
		return tcClient{}
	}
	c := f.counters[0]
	f.counters = f.counters[1:]
	return tcClient{counters: c}
}

type tcClient struct{ counters []NodeTrafficCounter }

func (c tcClient) GetTrafficCounters(ctx context.Context) ([]NodeTrafficCounter, error) {
	return c.counters, nil
}
