package invitecode

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("invite code not found")

type InviteCode struct {
	ID                 string
	Code               string
	Status             string
	MaxActivations     int
	CurrentActivations int
	ExhaustedAt        *time.Time
	ExpiresAt          *time.Time
	CreatedByUserID    *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Activation struct {
	ID           string
	InviteCodeID string
	UserID       string
	ActivatedAt  time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Repository interface {
	GetByCodeForUpdate(ctx context.Context, code string) (*InviteCode, error)
	IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*InviteCode, error)
	CreateActivation(ctx context.Context, in CreateActivationParams) (*Activation, error)
	HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error)
	Create(ctx context.Context, in CreateParams) (*InviteCode, error)
	ListRecent(ctx context.Context, limit int) ([]*InviteCode, error)
}

type CreateParams struct {
	Code            string
	Status          string
	MaxActivations  int
	ExpiresAt       *time.Time
	CreatedByUserID *string
}

type CreateActivationParams struct {
	InviteCodeID string
	UserID       string
}
