package app

import (
	"errors"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

func TestMinLoadNodeAssigner_AssignsLowestLoadWithinCapacity(t *testing.T) {
	assigner := MinLoadNodeAssigner{}

	got, err := assigner.Assign([]*node.Node{
		{ID: "n1", Status: node.StatusActive, CurrentLoad: 10, UserCapacity: 40, LastSeenAt: ptrTime(time.Now().UTC()), RuntimeMetadata: map[string]any{"protocols_supported": []any{"wireguard"}}},
		{ID: "n2", Status: node.StatusActive, CurrentLoad: 5, UserCapacity: 6, LastSeenAt: ptrTime(time.Now().UTC()), RuntimeMetadata: map[string]any{"protocols_supported": []any{"wireguard"}}},
		{ID: "n3", Status: node.StatusActive, CurrentLoad: 7, UserCapacity: 50, LastSeenAt: ptrTime(time.Now().UTC()), RuntimeMetadata: map[string]any{"protocols_supported": []any{"wireguard"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "n2" {
		t.Fatalf("expected n2, got %s", got.ID)
	}
}

func TestMinLoadNodeAssigner_ReturnsCapacityExceededWhenAllFull(t *testing.T) {
	assigner := MinLoadNodeAssigner{}

	_, err := assigner.Assign([]*node.Node{
		{ID: "n1", Status: node.StatusActive, CurrentLoad: 40, UserCapacity: 40, LastSeenAt: ptrTime(time.Now().UTC()), RuntimeMetadata: map[string]any{"protocols_supported": []any{"wireguard"}}},
		{ID: "n2", Status: node.StatusActive, CurrentLoad: 5, UserCapacity: 5, LastSeenAt: ptrTime(time.Now().UTC()), RuntimeMetadata: map[string]any{"protocols_supported": []any{"wireguard"}}},
	})
	if !errors.Is(err, ErrNodeCapacityExceeded) {
		t.Fatalf("expected ErrNodeCapacityExceeded, got %v", err)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }

func TestMinLoadNodeAssigner_ReturnsNoActiveNodes(t *testing.T) {
	assigner := MinLoadNodeAssigner{}

	_, err := assigner.Assign(nil)
	if !errors.Is(err, ErrNoActiveNodes) {
		t.Fatalf("expected ErrNoActiveNodes, got %v", err)
	}
}

func TestMinLoadNodeAssigner_SingleNodeModeAlwaysSelectsConfiguredNode(t *testing.T) {
	assigner := MinLoadNodeAssigner{SingleNodeID: "n-current"}
	got, err := assigner.Assign([]*node.Node{
		{ID: "stale", Status: node.StatusActive, CurrentLoad: 1, UserCapacity: 100},
		{ID: "n-current", Status: node.StatusActive, CurrentLoad: 99, UserCapacity: 100},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "n-current" {
		t.Fatalf("expected n-current, got %s", got.ID)
	}
}

func TestMinLoadNodeAssigner_SingleNodeModeRejectsMissingNode(t *testing.T) {
	assigner := MinLoadNodeAssigner{SingleNodeID: "n-current"}
	_, err := assigner.Assign([]*node.Node{{ID: "stale", Status: node.StatusActive}})
	if !errors.Is(err, ErrNoActiveNodes) {
		t.Fatalf("expected ErrNoActiveNodes, got %v", err)
	}
}
