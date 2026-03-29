package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
)

func TestExecuteCreatePeerOperation_Success(t *testing.T) {
	ops := &execFakeOps{op: &operation.NodeOperation{ID: "op1", VPNNodeID: "n1", PayloadJSON: `{"access_id":"a1","public_key":"pk","assigned_ip":"10.0.0.2/32","protocol":"wireguard","keepalive":25}`}}
	acc := &execFakeAccess{entry: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1", AssignedIP: strPtr("10.0.0.2/32")}}
	nodes := &execFakeNodes{node: &node.Node{ID: "n1", CurrentLoad: 5}}
	client := &execFakeNodeClient{}

	uc := ExecuteCreatePeerOperation{Operations: ops, Accesses: acc, Nodes: nodes, NodeClient: client, Now: func() time.Time { return time.Unix(100, 0) }}
	if err := uc.Execute(context.Background(), "op1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !acc.activated {
		t.Fatal("expected access activated")
	}
	if nodes.updatedLoad != 6 {
		t.Fatalf("expected updated load 6, got %d", nodes.updatedLoad)
	}
}

func TestExecuteCreatePeerOperation_FailureMarksError(t *testing.T) {
	ops := &execFakeOps{op: &operation.NodeOperation{ID: "op1", VPNNodeID: "n1", PayloadJSON: `{"access_id":"a1","public_key":"pk","assigned_ip":"10.0.0.2/32"}`}}
	acc := &execFakeAccess{entry: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1", AssignedIP: strPtr("10.0.0.2/32")}}
	nodes := &execFakeNodes{node: &node.Node{ID: "n1", CurrentLoad: 5}}
	client := &execFakeNodeClient{applyErr: errors.New("timeout")}

	uc := ExecuteCreatePeerOperation{Operations: ops, Accesses: acc, Nodes: nodes, NodeClient: client}
	if err := uc.Execute(context.Background(), "op1"); err == nil {
		t.Fatal("expected error")
	}
	if !acc.markedError {
		t.Fatal("expected access error status")
	}
}

func TestExecuteRevokePeerOperation_Success(t *testing.T) {
	ops := &execFakeOps{op: &operation.NodeOperation{ID: "op2", VPNNodeID: "n1"}}
	acc := &execFakeAccess{entry: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1", AssignedIP: strPtr("10.0.0.2/32")}}
	nodes := &execFakeNodes{node: &node.Node{ID: "n1", CurrentLoad: 3}}
	client := &execFakeNodeClient{}

	uc := ExecuteRevokePeerOperation{Operations: ops, Accesses: acc, Nodes: nodes, NodeClient: client, Now: func() time.Time { return time.Unix(100, 0) }}
	if err := uc.Execute(context.Background(), "op2", "a1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !acc.revoked {
		t.Fatal("expected access revoked")
	}
	if nodes.updatedLoad != 2 {
		t.Fatalf("expected updated load 2, got %d", nodes.updatedLoad)
	}
}

type execFakeOps struct{ op *operation.NodeOperation }

func (f *execFakeOps) GetByID(ctx context.Context, id string) (*operation.NodeOperation, error) {
	return f.op, nil
}
func (f *execFakeOps) Create(ctx context.Context, in operation.CreateParams) (*operation.NodeOperation, error) {
	return &operation.NodeOperation{ID: "new"}, nil
}
func (f *execFakeOps) MarkRunning(ctx context.Context, id string, startedAt time.Time) error {
	return nil
}
func (f *execFakeOps) MarkSuccess(ctx context.Context, id string, finishedAt time.Time) error {
	return nil
}
func (f *execFakeOps) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) error {
	return nil
}

type execFakeAccess struct {
	entry       *access.DeviceAccess
	activated   bool
	revoked     bool
	markedError bool
}

func (f *execFakeAccess) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return f.entry, nil
}
func (f *execFakeAccess) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	return f.entry, nil
}
func (f *execFakeAccess) Activate(ctx context.Context, id string, grantedAt time.Time) error {
	f.activated = true
	return nil
}
func (f *execFakeAccess) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	f.revoked = true
	return nil
}
func (f *execFakeAccess) MarkError(ctx context.Context, id string, failedAt time.Time, message string) error {
	f.markedError = true
	return nil
}
func (f *execFakeAccess) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	return nil
}
func (f *execFakeAccess) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return []*access.DeviceAccess{f.entry}, nil
}
func (f *execFakeAccess) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	return f.entry, nil
}

type execFakeNodes struct {
	node        *node.Node
	updatedLoad int
}

func (f *execFakeNodes) ListActive(ctx context.Context) ([]*node.Node, error) {
	return []*node.Node{f.node}, nil
}
func (f *execFakeNodes) GetByID(ctx context.Context, id string) (*node.Node, error) {
	return f.node, nil
}
func (f *execFakeNodes) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	return nil
}
func (f *execFakeNodes) UpdateLoad(ctx context.Context, id string, currentLoad int) error {
	f.updatedLoad = currentLoad
	return nil
}

type execFakeNodeClient struct {
	applyErr  error
	revokeErr error
}

func (f *execFakeNodeClient) ApplyPeer(ctx context.Context, req NodeAgentOperationRequest) error {
	return f.applyErr
}
func (f *execFakeNodeClient) RevokePeer(ctx context.Context, req NodeAgentOperationRequest) error {
	return f.revokeErr
}

func strPtr(v string) *string { return &v }
