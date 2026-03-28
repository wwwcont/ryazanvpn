package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	ErrOperationConflict = errors.New("operation id already used for another action")
	ErrNotImplemented    = errors.New("runtime operation is not implemented")
)

type PeerOperationRequest struct {
	OperationID    string            `json:"operation_id"`
	DeviceAccessID string            `json:"device_access_id"`
	Protocol       string            `json:"protocol"`
	PeerPublicKey  string            `json:"peer_public_key"`
	AssignedIP     string            `json:"assigned_ip"`
	Keepalive      int               `json:"keepalive"`
	EndpointMeta   map[string]string `json:"endpoint_metadata,omitempty"`
}

type PeerState struct {
	DeviceAccessID string
	Protocol       string
	PeerPublicKey  string
	AssignedIP     string
	Keepalive      int
	EndpointMeta   map[string]string
	UpdatedAt      time.Time
}

type OperationResult struct {
	OperationID string `json:"operation_id"`
	Applied     bool   `json:"applied"`
	Idempotent  bool   `json:"idempotent"`
}

type VPNRuntime interface {
	ApplyPeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error)
	RevokePeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error)
	Health(ctx context.Context) error
}
