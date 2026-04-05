package app

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

var ErrNoActiveNodes = errors.New("no active nodes available")
var ErrNodeCapacityExceeded = errors.New("node capacity exceeded")

type MinLoadNodeAssigner struct {
	SingleNodeID string
}

func (a MinLoadNodeAssigner) Assign(nodes []*node.Node) (*node.Node, error) {
	if selected, done, err := a.assignSingleNode(nodes); done {
		return selected, err
	}
	if len(nodes) == 0 {
		return nil, ErrNoActiveNodes
	}

	eligible := make([]*node.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Status != node.StatusActive {
			continue
		}
		if n.UserCapacity > 0 && n.CurrentLoad >= n.UserCapacity {
			continue
		}
		eligible = append(eligible, n)
	}
	if len(eligible) == 0 {
		return nil, ErrNodeCapacityExceeded
	}

	best := eligible[0]
	for i := 1; i < len(eligible); i++ {
		if eligible[i].CurrentLoad < best.CurrentLoad {
			best = eligible[i]
		}
	}
	return best, nil
}

func (a MinLoadNodeAssigner) assignSingleNode(nodes []*node.Node) (*node.Node, bool, error) {
	if strings.TrimSpace(a.SingleNodeID) == "" {
		return nil, false, nil
	}
	configuredID := strings.TrimSpace(a.SingleNodeID)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.ID != configuredID {
			if n.Status == node.StatusActive {
				slog.Warn("single_node.assign.rejected_stale", "configured_node_id", configuredID, "stale_node_id", n.ID)
			}
			continue
		}
		if n.Status != node.StatusActive {
			return nil, true, ErrNoActiveNodes
		}
		if n.UserCapacity > 0 && n.CurrentLoad >= n.UserCapacity {
			return nil, true, ErrNodeCapacityExceeded
		}
		slog.Info("single_node.assign.selected", "node_id", n.ID)
		return n, true, nil
	}
	return nil, true, ErrNoActiveNodes
}

func nodeSupportsProtocol(n *node.Node, protocol string) bool {
	if n == nil || strings.TrimSpace(protocol) == "" {
		return false
	}
	raw, ok := n.RuntimeMetadata["protocols_supported"]
	if !ok {
		return protocol == "wireguard"
	}
	switch v := raw.(type) {
	case []string:
		for _, item := range v {
			if strings.EqualFold(strings.TrimSpace(item), protocol) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(s), protocol) {
				return true
			}
		}
	}
	return false
}
