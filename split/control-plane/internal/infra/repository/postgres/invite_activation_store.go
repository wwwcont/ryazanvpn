package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

type InviteActivationStore struct {
	pool *pgxpool.Pool
}

func NewInviteActivationStore(pool *pgxpool.Pool) *InviteActivationStore {
	return &InviteActivationStore{pool: pool}
}

func (s *InviteActivationStore) WithinTx(ctx context.Context, fn func(repos app.ActivateInviteCodeRepos) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	repos := &inviteActivationRepos{
		users:        (&UserRepository{q: tx}),
		invites:      (&InviteCodeRepository{q: tx}),
		accessGrants: (&AccessGrantRepository{q: tx}),
		auditLogs:    (&AuditLogRepository{q: tx}),
	}

	if err := fn(repos); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type inviteActivationRepos struct {
	users        user.Repository
	invites      invitecode.Repository
	accessGrants accessgrant.Repository
	auditLogs    audit.Repository
}

func (r *inviteActivationRepos) Users() user.Repository {
	return r.users
}

func (r *inviteActivationRepos) InviteCodes() invitecode.Repository {
	return r.invites
}

func (r *inviteActivationRepos) AccessGrants() accessgrant.Repository {
	return r.accessGrants
}

func (r *inviteActivationRepos) AuditLogs() audit.Repository {
	return r.auditLogs
}
