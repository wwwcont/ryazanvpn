package audit

import (
	"context"
	"time"
)

type Log struct {
	ID          string
	ActorUserID *string
	EntityType  string
	EntityID    *string
	Action      string
	DetailsJSON string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Repository interface {
	Create(ctx context.Context, in CreateParams) (*Log, error)
}

type CreateParams struct {
	ActorUserID *string
	EntityType  string
	EntityID    *string
	Action      string
	DetailsJSON string
}
