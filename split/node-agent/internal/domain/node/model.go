package node

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("vpn node not found")

type Node struct {
	ID              string
	Name            string
	Region          string
	AgentBaseURL    string
	VPNEndpoint     string
	VPNEndpointHost string
	VPNEndpointPort int
	ServerPublicKey string
	VPNSubnetCIDR   string
	RuntimeMetadata map[string]any
	Status          string
	CurrentLoad     int
	UserCapacity    int
	LastSeenAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Repository interface {
	ListAll(ctx context.Context) ([]*Node, error)
	ListActive(ctx context.Context) ([]*Node, error)
	GetByID(ctx context.Context, id string) (*Node, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error
	UpdateLoad(ctx context.Context, id string, currentLoad int) error
}
