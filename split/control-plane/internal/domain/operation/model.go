package operation

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("node operation not found")

type NodeOperation struct {
	ID                string
	VPNNodeID         string
	OperationType     string
	Status            string
	RequestedByUserID *string
	PayloadJSON       string
	ErrorMessage      *string
	StartedAt         *time.Time
	FinishedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Repository interface {
	GetByID(ctx context.Context, id string) (*NodeOperation, error)
	Create(ctx context.Context, in CreateParams) (*NodeOperation, error)
	MarkRunning(ctx context.Context, id string, startedAt time.Time) error
	MarkSuccess(ctx context.Context, id string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) error
}

type CreateParams struct {
	VPNNodeID         string
	OperationType     string
	Status            string
	RequestedByUserID *string
	PayloadJSON       string
}
