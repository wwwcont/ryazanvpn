package app

import (
	"errors"

	"github.com/example/ryazanvpn/internal/domain/node"
)

var ErrNoActiveNodes = errors.New("no active nodes available")

type MinLoadNodeAssigner struct{}

func (a MinLoadNodeAssigner) Assign(nodes []*node.Node) (*node.Node, error) {
	if len(nodes) == 0 {
		return nil, ErrNoActiveNodes
	}

	best := nodes[0]
	for i := 1; i < len(nodes); i++ {
		if nodes[i].CurrentLoad < best.CurrentLoad {
			best = nodes[i]
		}
	}
	return best, nil
}
