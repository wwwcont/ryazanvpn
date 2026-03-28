package app

import (
	"errors"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

var ErrNoActiveNodes = errors.New("no active nodes available")
var ErrNodeCapacityExceeded = errors.New("node capacity exceeded")

type MinLoadNodeAssigner struct{}

func (a MinLoadNodeAssigner) Assign(nodes []*node.Node) (*node.Node, error) {
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
