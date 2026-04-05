package access

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("device access not found")

type DeviceAccess struct {
	ID                  string
	DeviceID            string
	VPNNodeID           string
	Protocol            string
	Status              string
	AssignedIP          *string
	PresharedKey        *string
	ConfigBlobEncrypted []byte
	GrantedAt           *time.Time
	RevokedAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Repository interface {
	Create(ctx context.Context, in CreateParams) (*DeviceAccess, error)
	GetByID(ctx context.Context, id string) (*DeviceAccess, error)
	Activate(ctx context.Context, id string, grantedAt time.Time) error
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
	MarkError(ctx context.Context, id string, failedAt time.Time, message string) error
	SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error
	ClearConfigBlobEncrypted(ctx context.Context, id string) error
	GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*DeviceAccess, error)
	GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*DeviceAccess, error)
	GetActiveByNodeAndPublicKey(ctx context.Context, nodeID string, publicKey string) (*DeviceAccess, error)
	ListActiveByNodeID(ctx context.Context, nodeID string) ([]*DeviceAccess, error)
}

type CreateParams struct {
	DeviceID     string
	VPNNodeID    string
	Protocol     string
	Status       string
	AssignedIP   *string
	PresharedKey *string
}
