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

type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (*user.User, error)
	GetByUsername(ctx context.Context, username string) (*user.User, error)
	Create(ctx context.Context, in user.CreateParams) (*user.User, error)
	GetByID(ctx context.Context, id string) (*user.User, error)
}

type InviteCodeRepository interface {
	GetByCodeForUpdate(ctx context.Context, code string) (*invitecode.InviteCode, error)
	IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*invitecode.InviteCode, error)
	CreateActivation(ctx context.Context, in invitecode.CreateActivationParams) (*invitecode.Activation, error)
	HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error)
	Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error)
	ListRecent(ctx context.Context, limit int) ([]*invitecode.InviteCode, error)
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

type AccessGrantRepository interface {
	Create(ctx context.Context, in accessgrant.CreateParams) (*accessgrant.AccessGrant, error)
	GetActiveByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error)
	GetLatestByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error)
	ExpireActiveBefore(ctx context.Context, now time.Time) (int64, error)
	ListActiveUsers(ctx context.Context, limit int) ([]*accessgrant.ActiveUserGrant, error)
	RevokeActiveByUserID(ctx context.Context, userID string) (int64, error)
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

type TrafficRepository interface {
	CreateSnapshot(ctx context.Context, in traffic.CreateSnapshotParams) (*traffic.DeviceTrafficSnapshot, error)
	GetLastSnapshotByDeviceID(ctx context.Context, deviceID string, before time.Time) (*traffic.DeviceTrafficSnapshot, error)
	AddDailyUsageDelta(ctx context.Context, in traffic.AddDailyUsageDeltaParams) error
	GetDeviceTrafficTotal(ctx context.Context, deviceID string) (int64, error)
	GetDeviceTrafficLastNDays(ctx context.Context, deviceID string, days int, now time.Time) (int64, error)
	GetUserTrafficTotal(ctx context.Context, userID string) (int64, error)
	GetUserTrafficLastNDays(ctx context.Context, userID string, days int, now time.Time) (int64, error)
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

type NodeTrafficClient interface {
	GetTrafficCounters(ctx context.Context) ([]NodeTrafficCounter, error)
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

type NodeTrafficCounter struct {
	DeviceAccessID  string
	RXTotalBytes    int64
	TXTotalBytes    int64
	LastHandshakeAt *time.Time
}
