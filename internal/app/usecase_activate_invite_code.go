package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	UserID         string
	Code           string
	TelegramUserID int64
	TelegramChatID int64
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
	Store   ActivateInviteCodeStore
	Now     func() time.Time
	Finance *FinanceService
}

func (uc ActivateInviteCode) Execute(ctx context.Context, in ActivateInviteCodeInput) error {
	startedAt := time.Now()
	baseFields := []any{
		"telegram_user_id", in.TelegramUserID,
		"chat_id", in.TelegramChatID,
		"user_id", in.UserID,
		"invite_code", in.Code,
	}
	slog.Info("activate_invite_code.start", append(baseFields, "duration_ms", int64(0))...)
	if !inviteCodeRegex.MatchString(in.Code) {
		slog.Error("activate_invite_code.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", ErrInvalidInviteCodeFormat)...)
		return ErrInvalidInviteCodeFormat
	}

	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	err := uc.Store.WithinTx(ctx, func(repos ActivateInviteCodeRepos) error {
		slog.Info("activate_invite_code.lookup_invite.start", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)
		u, err := repos.Users().GetByID(ctx, in.UserID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				return err
			}
			return err
		}

		ic, err := repos.InviteCodes().GetByCodeForUpdate(ctx, in.Code)
		if err != nil {
			slog.Error("activate_invite_code.lookup_invite.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)...)
			return err
		}
		slog.Info("activate_invite_code.lookup_invite.success", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)

		slog.Info("activate_invite_code.lock_user.start", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)
		if activeGrant, err := repos.AccessGrants().GetActiveByUserID(ctx, u.ID); err == nil && activeGrant != nil {
			slog.Error("activate_invite_code.lock_user.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", ErrUserAlreadyHasActiveAccessGrant)...)
			return ErrUserAlreadyHasActiveAccessGrant
		} else if err != nil && !errors.Is(err, accessgrant.ErrNotFound) {
			slog.Error("activate_invite_code.lock_user.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)...)
			return err
		}
		slog.Info("activate_invite_code.lock_user.success", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)

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

		slog.Info("activate_invite_code.create_grant.start", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)
		accessGrant, err := repos.AccessGrants().Create(ctx, accessgrant.CreateParams{
			UserID:       u.ID,
			InviteCodeID: ic.ID,
			Status:       accessgrant.StatusActive,
			ActivatedAt:  now,
			ExpiresAt:    now.Add(accessGrantDuration),
			DevicesLimit: 1,
		})
		if err != nil {
			slog.Error("activate_invite_code.create_grant.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)...)
			return err
		}
		if uc.Finance != nil {
			if err := uc.Finance.ApplyInviteBonus(ctx, u.ID, ic.ID, 20_000); err != nil {
				return err
			}
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
		slog.Info("activate_invite_code.create_grant.success", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "grant_id", accessGrant.ID)...)
		slog.Info("activate_invite_code.commit.start", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)

		return nil
	})
	if err != nil {
		slog.Error("activate_invite_code.commit.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)...)
		slog.Error("activate_invite_code.error", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)...)
		return err
	}
	slog.Info("activate_invite_code.commit.success", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)
	slog.Info("activate_invite_code.success", append(baseFields, "duration_ms", time.Since(startedAt).Milliseconds())...)
	return nil
}
