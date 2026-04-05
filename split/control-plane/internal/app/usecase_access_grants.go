package app

import (
	"context"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
)

type GetActiveAccessGrantByUserInput struct {
	UserID string
}

type GetActiveAccessGrantByUser struct {
	AccessGrants AccessGrantRepository
}

func (uc GetActiveAccessGrantByUser) Execute(ctx context.Context, in GetActiveAccessGrantByUserInput) (*accessgrant.AccessGrant, error) {
	return uc.AccessGrants.GetActiveByUserID(ctx, in.UserID)
}

type ExpireAccessGrants struct {
	AccessGrants AccessGrantRepository
	Now          func() time.Time
}

func (uc ExpireAccessGrants) Execute(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	return uc.AccessGrants.ExpireActiveBefore(ctx, now)
}
