package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
)

type AccessGrantRepository struct {
	q dbtx
}

func NewAccessGrantRepository(pool *pgxpool.Pool) *AccessGrantRepository {
	return &AccessGrantRepository{q: pool}
}

func (r *AccessGrantRepository) WithTx(tx pgx.Tx) *AccessGrantRepository {
	return &AccessGrantRepository{q: tx}
}

func (r *AccessGrantRepository) Create(ctx context.Context, in accessgrant.CreateParams) (*accessgrant.AccessGrant, error) {
	const query = `
INSERT INTO access_grants (user_id, invite_code_id, status, activated_at, expires_at, devices_limit)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, user_id::text, invite_code_id::text, status, activated_at, expires_at, devices_limit, created_at, updated_at`

	var out accessgrant.AccessGrant
	err := r.q.QueryRow(ctx, query,
		in.UserID,
		in.InviteCodeID,
		in.Status,
		in.ActivatedAt,
		in.ExpiresAt,
		in.DevicesLimit,
	).Scan(
		&out.ID,
		&out.UserID,
		&out.InviteCodeID,
		&out.Status,
		&out.ActivatedAt,
		&out.ExpiresAt,
		&out.DevicesLimit,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AccessGrantRepository) GetActiveByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	const query = `
SELECT id::text, user_id::text, invite_code_id::text, status, activated_at, expires_at, devices_limit, created_at, updated_at
FROM access_grants
WHERE user_id = $1 AND status = $2
ORDER BY expires_at DESC
LIMIT 1`

	var out accessgrant.AccessGrant
	err := r.q.QueryRow(ctx, query, userID, accessgrant.StatusActive).Scan(
		&out.ID,
		&out.UserID,
		&out.InviteCodeID,
		&out.Status,
		&out.ActivatedAt,
		&out.ExpiresAt,
		&out.DevicesLimit,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, accessgrant.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AccessGrantRepository) GetLatestByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	const query = `
SELECT id::text, user_id::text, invite_code_id::text, status, activated_at, expires_at, devices_limit, created_at, updated_at
FROM access_grants
WHERE user_id = $1
ORDER BY activated_at DESC
LIMIT 1`

	var out accessgrant.AccessGrant
	err := r.q.QueryRow(ctx, query, userID).Scan(
		&out.ID,
		&out.UserID,
		&out.InviteCodeID,
		&out.Status,
		&out.ActivatedAt,
		&out.ExpiresAt,
		&out.DevicesLimit,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, accessgrant.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AccessGrantRepository) ExpireActiveBefore(ctx context.Context, now time.Time) (int64, error) {
	const query = `
UPDATE access_grants
SET status = $1, updated_at = NOW()
WHERE status = $2 AND expires_at <= $3`

	cmdTag, err := r.q.Exec(ctx, query, accessgrant.StatusExpired, accessgrant.StatusActive, now)
	if err != nil {
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}

func (r *AccessGrantRepository) ListActiveUsers(ctx context.Context, limit int) ([]*accessgrant.ActiveUserGrant, error) {
	if limit <= 0 {
		limit = 50
	}
	const query = `
SELECT ag.user_id::text, u.telegram_id, COALESCE(u.username, ''), ag.id::text, ag.status, ag.expires_at
FROM access_grants ag
JOIN users u ON u.id = ag.user_id
WHERE ag.status = $1
ORDER BY ag.expires_at ASC
LIMIT $2`

	rows, err := r.q.Query(ctx, query, accessgrant.StatusActive, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*accessgrant.ActiveUserGrant
	for rows.Next() {
		var item accessgrant.ActiveUserGrant
		if err := rows.Scan(&item.UserID, &item.TelegramID, &item.Username, &item.GrantID, &item.GrantStatus, &item.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *AccessGrantRepository) RevokeActiveByUserID(ctx context.Context, userID string) (int64, error) {
	const query = `
UPDATE access_grants
SET status = $1, updated_at = NOW()
WHERE user_id = $2 AND status = $3`

	cmd, err := r.q.Exec(ctx, query, accessgrant.StatusRevoked, userID, accessgrant.StatusActive)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}
