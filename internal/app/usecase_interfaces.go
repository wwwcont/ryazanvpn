package app

import (
	"context"
	"time"

	"github.com/example/ryazanvpn/internal/domain/access"
	"github.com/example/ryazanvpn/internal/domain/audit"
	"github.com/example/ryazanvpn/internal/domain/device"
	"github.com/example/ryazanvpn/internal/domain/invitecode"
	"github.com/example/ryazanvpn/internal/domain/node"
	"github.com/example/ryazanvpn/internal/domain/operation"
	"github.com/example/ryazanvpn/internal/domain/token"
	"github.com/example/ryazanvpn/internal/domain/user"
)

type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (*user.User, error)
	Create(ctx context.Context, in user.CreateParams) (*user.User, error)
	GetByID(ctx context.Context, id string) (*user.User, error)
}

type InviteCodeRepository interface {
	GetByCodeForUpdate(ctx context.Context, code string) (*invitecode.InviteCode, error)
	IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*invitecode.InviteCode, error)
	CreateActivation(ctx context.Context, in invitecode.CreateActivationParams) (*invitecode.Activation, error)
	HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error)
}

type NodeRepository interface {
	ListActive(ctx context.Context) ([]*node.Node, error)
	GetByID(ctx context.Context, id string) (*node.Node, error)
	UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error
	UpdateLoad(ctx context.Context, id string, currentLoad int) error
}

type DeviceRepository interface {
	Create(ctx context.Context, in device.CreateParams) (*device.Device, error)
	ListByUserID(ctx context.Context, userID string) ([]*device.Device, error)
	GetActiveByUserID(ctx context.Context, userID string) (*device.Device, error)
	Revoke(ctx context.Context, id string) error
	AssignNode(ctx context.Context, id string, vpnNodeID string) error
}

type DeviceAccessRepository interface {
	Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error)
	GetByID(ctx context.Context, id string) (*access.DeviceAccess, error)
	Activate(ctx context.Context, id string, grantedAt time.Time) error
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
	MarkError(ctx context.Context, id string, failedAt time.Time, message string) error
	SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error
	GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error)
}

type NodeOperationRepository interface {
	GetByID(ctx context.Context, id string) (*operation.NodeOperation, error)
	Create(ctx context.Context, in operation.CreateParams) (*operation.NodeOperation, error)
	MarkRunning(ctx context.Context, id string, startedAt time.Time) error
	MarkSuccess(ctx context.Context, id string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) error
}

type AuditLogRepository interface {
	Create(ctx context.Context, in audit.CreateParams) (*audit.Log, error)
}

type ConfigDownloadTokenRepository interface {
	Create(ctx context.Context, in token.CreateParams) (*token.ConfigDownloadToken, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*token.ConfigDownloadToken, error)
	MarkUsed(ctx context.Context, id string, consumedAt time.Time) error
}

type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type KeyGenerator interface {
	Generate(ctx context.Context) (publicKey string, privateKey string, err error)
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

type NodeAgentOperationRequest struct {
	OperationID    string
	DeviceAccessID string
	Protocol       string
	PeerPublicKey  string
	AssignedIP     string
	Keepalive      int
	EndpointMeta   map[string]string
}
