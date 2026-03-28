package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

var (
	ErrInvalidInviteCodeFormat         = errors.New("invite code must be 4 digits")
	ErrUserAlreadyHasActiveAccessGrant = errors.New("user already has active access grant")
)

const accessGrantDuration = 30 * 24 * time.Hour

var inviteCodeRegex = regexp.MustCompile(`^\d{4}$`)

type ActivateInviteCodeInput struct {
	UserID string
	Code   string
}

type ActivateInviteCodeRepos interface {
	Users() user.Repository
	InviteCodes() invitecode.Repository
	AccessGrants() accessgrant.Repository
	AuditLogs() audit.Repository
}

type ActivateInviteCodeStore interface {
	WithinTx(ctx context.Context, fn func(repos ActivateInviteCodeRepos) error) error
}

type ActivateInviteCode struct {
	Store ActivateInviteCodeStore
	Now   func() time.Time
}

func (uc ActivateInviteCode) Execute(ctx context.Context, in ActivateInviteCodeInput) error {
	if !inviteCodeRegex.MatchString(in.Code) {
		return ErrInvalidInviteCodeFormat
	}

	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	return uc.Store.WithinTx(ctx, func(repos ActivateInviteCodeRepos) error {
		u, err := repos.Users().GetByID(ctx, in.UserID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				return err
			}
			return err
		}

		ic, err := repos.InviteCodes().GetByCodeForUpdate(ctx, in.Code)
		if err != nil {
			return err
		}

		if activeGrant, err := repos.AccessGrants().GetActiveByUserID(ctx, u.ID); err == nil && activeGrant != nil {
			return ErrUserAlreadyHasActiveAccessGrant
		} else if err != nil && !errors.Is(err, accessgrant.ErrNotFound) {
			return err
		}

		if ic.Status != invitecode.CodeStatusActive {
			return ErrInviteCodeNotUsable
		}
		if ic.ExpiresAt != nil && ic.ExpiresAt.Before(now) {
			return ErrInviteCodeNotUsable
		}
		if ic.CurrentActivations >= ic.MaxActivations {
			return ErrInviteCodeNotUsable
		}

		has, err := repos.InviteCodes().HasActivationByUser(ctx, ic.ID, u.ID)
		if err != nil {
			return err
		}
		if has {
			return ErrInviteAlreadyUsed
		}

		if _, err := repos.InviteCodes().CreateActivation(ctx, invitecode.CreateActivationParams{InviteCodeID: ic.ID, UserID: u.ID}); err != nil {
			return err
		}

		accessGrant, err := repos.AccessGrants().Create(ctx, accessgrant.CreateParams{
			UserID:       u.ID,
			InviteCodeID: ic.ID,
			Status:       accessgrant.StatusActive,
			ActivatedAt:  now,
			ExpiresAt:    now.Add(accessGrantDuration),
			DevicesLimit: 1,
		})
		if err != nil {
			return err
		}

		updated, err := repos.InviteCodes().IncrementUsageAndMaybeExhaust(ctx, ic.ID)
		if err != nil {
			return err
		}

		details, _ := json.Marshal(map[string]any{
			"invite_code_id":      ic.ID,
			"invite_code":         ic.Code,
			"access_grant_id":     accessGrant.ID,
			"access_expires_at":   accessGrant.ExpiresAt,
			"status_after_update": updated.Status,
			"used_count":          updated.CurrentActivations,
			"max_uses":            updated.MaxActivations,
		})

		_, err = repos.AuditLogs().Create(ctx, audit.CreateParams{
			ActorUserID: &u.ID,
			EntityType:  "invite_code",
			EntityID:    &ic.ID,
			Action:      "activate_invite_code",
			DetailsJSON: string(details),
		})
		if err != nil {
			return fmt.Errorf("create audit log: %w", err)
		}

		return nil
	})
}
