package runtime

import (
	"context"
	"errors"
	"time"
)

var (
	ErrOperationConflict = errors.New("operation id already used for another action")
	ErrNotImplemented    = errors.New("runtime operation is not implemented")
	ErrUnavailable       = errors.New("runtime unavailable")
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

type PeerStat struct {
	DeviceAccessID          string     `json:"device_access_id,omitempty"`
	Protocol                string     `json:"protocol,omitempty"`
	PeerPublicKey           string     `json:"peer_public_key,omitempty"`
	PresharedKey            string     `json:"preshared_key,omitempty"`
	Endpoint                string     `json:"endpoint,omitempty"`
	AllowedIP               string     `json:"allowed_ip,omitempty"`
	LatestHandshakeUnixTime int64      `json:"latest_handshake_unix_timestamp,omitempty"`
	RXTotalBytes            int64      `json:"rx_total_bytes"`
	TXTotalBytes            int64      `json:"tx_total_bytes"`
	LastHandshakeAt         *time.Time `json:"last_handshake_at,omitempty"`
}

type PeerManager interface {
	ApplyPeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error)
	RevokePeer(ctx context.Context, req PeerOperationRequest) (OperationResult, error)
}

type PeerStatsReader interface {
	ListPeerStats(ctx context.Context) ([]PeerStat, error)
}

type VPNRuntime interface {
	PeerManager
	PeerStatsReader
	Health(ctx context.Context) error
}
