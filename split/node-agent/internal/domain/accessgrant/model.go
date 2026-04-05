package accessgrant

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("access grant not found")

type AccessGrant struct {
	ID           string
	UserID       string
	InviteCodeID string
	Status       string
	ActivatedAt  time.Time
	ExpiresAt    time.Time
	DevicesLimit int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Repository interface {
	Create(ctx context.Context, in CreateParams) (*AccessGrant, error)
	GetActiveByUserID(ctx context.Context, userID string) (*AccessGrant, error)
	GetLatestByUserID(ctx context.Context, userID string) (*AccessGrant, error)
	ExpireActiveBefore(ctx context.Context, now time.Time) (int64, error)
	ListActiveUsers(ctx context.Context, limit int) ([]*ActiveUserGrant, error)
	RevokeActiveByUserID(ctx context.Context, userID string) (int64, error)
}

type ActiveUserGrant struct {
	UserID      string
	TelegramID  int64
	Username    string
	GrantID     string
	GrantStatus string
	ExpiresAt   time.Time
}

type CreateParams struct {
	UserID       string
	InviteCodeID string
	Status       string
	ActivatedAt  time.Time
	ExpiresAt    time.Time
	DevicesLimit int
}
