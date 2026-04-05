package device

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID         string
	UserID     string
	VPNNodeID  *string
	PublicKey  string
	Name       string
	Platform   string
	Status     string
	LastSeenAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Repository interface {
	Create(ctx context.Context, in CreateParams) (*Device, error)
	ListByUserID(ctx context.Context, userID string) ([]*Device, error)
	GetByID(ctx context.Context, id string) (*Device, error)
	GetActiveByUserID(ctx context.Context, userID string) (*Device, error)
	Revoke(ctx context.Context, id string) error
	AssignNode(ctx context.Context, id string, vpnNodeID string) error
}

type CreateParams struct {
	UserID    string
	VPNNodeID *string
	PublicKey string
	Name      string
	Platform  string
	Status    string
}
