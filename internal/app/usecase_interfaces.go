package app

import (
	"context"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

type UserRepository = user.Repository

type InviteCodeRepository = invitecode.Repository

type NodeRepository = node.Repository

type DeviceRepository = device.Repository

type DeviceAccessRepository = access.Repository

type AccessGrantRepository = accessgrant.Repository

type NodeOperationRepository = operation.Repository

type AuditLogRepository = audit.Repository

type ConfigDownloadTokenRepository = token.Repository

type TrafficRepository = traffic.Repository

type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type KeyGenerator interface {
	Generate(ctx context.Context) (publicKey string, privateKey string, err error)
}

type PresharedKeyGenerator interface {
	GeneratePresharedKey(ctx context.Context) (string, error)
}

type IPAllocator interface {
	Allocate(ctx context.Context, nodeID string) (string, error)
}

type NodeAssigner interface {
	Assign(nodes []*node.Node) (*node.Node, error)
}

type NodeAgentClient interface {
	ApplyPeer(ctx context.Context, req NodeAgentOperationRequest) error
	RevokePeer(ctx context.Context, req NodeAgentOperationRequest) error
}

type NodeTrafficClient interface {
	GetTrafficCounters(ctx context.Context) ([]NodeTrafficCounter, error)
}

type NodeAgentOperationRequest struct {
	AgentBaseURL   string
	OperationID    string
	DeviceAccessID string
	Protocol       string
	PeerPublicKey  string
	AssignedIP     string
	Keepalive      int
	EndpointMeta   map[string]string
}

type NodeTrafficCounter struct {
	DeviceAccessID  string
	Protocol        string
	PeerPublicKey   string
	AllowedIP       string
	Endpoint        string
	PresharedKey    string
	RXTotalBytes    int64
	TXTotalBytes    int64
	LastHandshakeAt *time.Time
}
