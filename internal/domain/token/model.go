package token

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("config download token not found")

type ConfigDownloadToken struct {
	ID             string
	DeviceAccessID string
	TokenHash      string
	Status         string
	ExpiresAt      time.Time
	UsedAt         *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Repository interface {
	Create(ctx context.Context, in CreateParams) (*ConfigDownloadToken, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*ConfigDownloadToken, error)
	MarkUsed(ctx context.Context, id string, consumedAt time.Time) error
	RevokeIssuedByAccessID(ctx context.Context, deviceAccessID string, revokedAt time.Time) error
}

type CreateParams struct {
	DeviceAccessID string
	TokenHash      string
	Status         string
	ExpiresAt      time.Time
}
