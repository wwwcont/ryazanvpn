package app

import (
	"context"
	"errors"

	"github.com/example/ryazanvpn/internal/domain/invitecode"
	"github.com/example/ryazanvpn/internal/domain/user"
)

var (
	ErrInviteCodeNotUsable = errors.New("invite code is not usable")
	ErrInviteAlreadyUsed   = errors.New("invite code already used by user")
)

type InviteActivationStore interface {
	WithinTx(ctx context.Context, fn func(repos InviteActivationRepos) error) error
}

type InviteActivationRepos interface {
	Users() user.Repository
	InviteCodes() invitecode.Repository
}

type InviteActivationService struct {
	Store InviteActivationStore
}

func (s *InviteActivationService) ActivateByTelegramID(ctx context.Context, telegramID int64, code string) error {
	return s.Store.WithinTx(ctx, func(repos InviteActivationRepos) error {
		u, err := repos.Users().GetByTelegramID(ctx, telegramID)
		if err != nil {
			return err
		}

		ic, err := repos.InviteCodes().GetByCodeForUpdate(ctx, code)
		if err != nil {
			return err
		}

		if ic.Status != invitecode.CodeStatusActive {
			return ErrInviteCodeNotUsable
		}
		if ic.CurrentActivations >= ic.MaxActivations {
			return ErrInviteCodeNotUsable
		}

		hasActivation, err := repos.InviteCodes().HasActivationByUser(ctx, ic.ID, u.ID)
		if err != nil {
			return err
		}
		if hasActivation {
			return ErrInviteAlreadyUsed
		}

		if _, err := repos.InviteCodes().CreateActivation(ctx, invitecode.CreateActivationParams{
			InviteCodeID: ic.ID,
			UserID:       u.ID,
		}); err != nil {
			return err
		}

		_, err = repos.InviteCodes().IncrementUsageAndMaybeExhaust(ctx, ic.ID)
		return err
	})
}
