package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
)

type InviteCodeRepository struct {
	q dbtx
}

func NewInviteCodeRepository(pool *pgxpool.Pool) *InviteCodeRepository {
	return &InviteCodeRepository{q: pool}
}

func (r *InviteCodeRepository) WithTx(tx pgx.Tx) *InviteCodeRepository {
	return &InviteCodeRepository{q: tx}
}

func (r *InviteCodeRepository) GetByCodeForUpdate(ctx context.Context, code string) (*invitecode.InviteCode, error) {
	const query = `
SELECT id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
       created_by_user_id::text, created_at, updated_at
FROM invite_codes
WHERE code = $1
FOR UPDATE`

	out, err := scanInviteCode(r.q.QueryRow(ctx, query, code))
	if isNoRows(err) {
		return nil, invitecode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InviteCodeRepository) IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*invitecode.InviteCode, error) {
	const query = `
UPDATE invite_codes
SET
  current_activations = current_activations + 1,
  status = CASE
    WHEN current_activations + 1 >= max_activations THEN 'exhausted'
    ELSE status
  END,
  exhausted_at = CASE
    WHEN current_activations + 1 >= max_activations THEN NOW()
    ELSE exhausted_at
  END,
  updated_at = NOW()
WHERE id = $1
RETURNING id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
          created_by_user_id::text, created_at, updated_at`

	out, err := scanInviteCode(r.q.QueryRow(ctx, query, inviteCodeID))
	if isNoRows(err) {
		return nil, invitecode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InviteCodeRepository) CreateActivation(ctx context.Context, in invitecode.CreateActivationParams) (*invitecode.Activation, error) {
	const query = `
INSERT INTO invite_code_activations (invite_code_id, user_id, activated_at)
VALUES ($1, $2, NOW())
RETURNING id::text, invite_code_id::text, user_id::text, activated_at, created_at, updated_at`

	var out invitecode.Activation
	err := r.q.QueryRow(ctx, query, in.InviteCodeID, in.UserID).Scan(
		&out.ID,
		&out.InviteCodeID,
		&out.UserID,
		&out.ActivatedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *InviteCodeRepository) HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error) {
	const query = `
SELECT EXISTS (
  SELECT 1
  FROM invite_code_activations
  WHERE invite_code_id = $1 AND user_id = $2
)`

	var exists bool
	if err := r.q.QueryRow(ctx, query, inviteCodeID, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *InviteCodeRepository) GetByID(ctx context.Context, id string) (*invitecode.InviteCode, error) {
	const query = `
SELECT id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
       created_by_user_id::text, created_at, updated_at
FROM invite_codes
WHERE id = $1`

	out, err := scanInviteCode(r.q.QueryRow(ctx, query, id))
	if isNoRows(err) {
		return nil, invitecode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InviteCodeRepository) RevokeByID(ctx context.Context, id string) (*invitecode.InviteCode, error) {
	const query = `
UPDATE invite_codes
SET status = $2, updated_at = NOW()
WHERE id = $1 AND status <> $2
RETURNING id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
          created_by_user_id::text, created_at, updated_at`

	out, err := scanInviteCode(r.q.QueryRow(ctx, query, id, invitecode.CodeStatusRevoked))
	if isNoRows(err) {
		return r.GetByID(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InviteCodeRepository) Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error) {
	const query = `
INSERT INTO invite_codes (code, status, max_activations, current_activations, exhausted_at, expires_at, created_by_user_id)
VALUES ($1, $2, $3, 0, NULL, $4, $5)
RETURNING id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
          created_by_user_id::text, created_at, updated_at`

	out, err := scanInviteCode(r.q.QueryRow(ctx, query, in.Code, in.Status, in.MaxActivations, in.ExpiresAt, in.CreatedByUserID))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InviteCodeRepository) ListRecent(ctx context.Context, limit int) ([]*invitecode.InviteCode, error) {
	if limit <= 0 {
		limit = 20
	}
	const query = `
SELECT id::text, code, status, max_activations, current_activations, exhausted_at, expires_at,
       created_by_user_id::text, created_at, updated_at
FROM invite_codes
ORDER BY created_at DESC
LIMIT $1`

	rows, err := r.q.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*invitecode.InviteCode
	for rows.Next() {
		item, err := scanInviteCode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanInviteCode(row pgx.Row) (*invitecode.InviteCode, error) {
	var out invitecode.InviteCode
	var exhaustedAt *time.Time
	var expiresAt *time.Time
	var createdByID *string

	err := row.Scan(
		&out.ID,
		&out.Code,
		&out.Status,
		&out.MaxActivations,
		&out.CurrentActivations,
		&exhaustedAt,
		&expiresAt,
		&createdByID,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	out.ExhaustedAt = exhaustedAt
	out.ExpiresAt = expiresAt
	out.CreatedByUserID = createdByID

	return &out, nil
}
