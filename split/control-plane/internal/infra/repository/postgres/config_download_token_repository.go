package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
)

type ConfigDownloadTokenRepository struct {
	q dbtx
}

func NewConfigDownloadTokenRepository(pool *pgxpool.Pool) *ConfigDownloadTokenRepository {
	return &ConfigDownloadTokenRepository{q: pool}
}

func (r *ConfigDownloadTokenRepository) WithTx(tx pgx.Tx) *ConfigDownloadTokenRepository {
	return &ConfigDownloadTokenRepository{q: tx}
}

func (r *ConfigDownloadTokenRepository) Create(ctx context.Context, in token.CreateParams) (*token.ConfigDownloadToken, error) {
	const query = `
INSERT INTO config_download_tokens (device_access_id, token_hash, status, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id::text, device_access_id::text, token_hash, status, expires_at, used_at, created_at, updated_at`

	out, err := scanToken(r.q.QueryRow(ctx, query, in.DeviceAccessID, in.TokenHash, in.Status, in.ExpiresAt))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ConfigDownloadTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*token.ConfigDownloadToken, error) {
	const query = `
SELECT id::text, device_access_id::text, token_hash, status, expires_at, used_at, created_at, updated_at
FROM config_download_tokens
WHERE token_hash = $1`

	out, err := scanToken(r.q.QueryRow(ctx, query, tokenHash))
	if isNoRows(err) {
		return nil, token.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ConfigDownloadTokenRepository) MarkUsed(ctx context.Context, id string, consumedAt time.Time) error {
	const query = `
UPDATE config_download_tokens
SET status = 'used', used_at = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, consumedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return token.ErrNotFound
	}
	return nil
}

func (r *ConfigDownloadTokenRepository) RevokeIssuedByAccessID(ctx context.Context, deviceAccessID string, revokedAt time.Time) error {
	const query = `
UPDATE config_download_tokens
SET status = 'revoked', updated_at = NOW()
WHERE device_access_id = $1
  AND status = 'issued'
  AND (used_at IS NULL)
  AND expires_at >= $2`

	_, err := r.q.Exec(ctx, query, deviceAccessID, revokedAt)
	return err
}

func scanToken(row pgx.Row) (*token.ConfigDownloadToken, error) {
	var out token.ConfigDownloadToken
	var consumedAt *time.Time
	err := row.Scan(
		&out.ID,
		&out.DeviceAccessID,
		&out.TokenHash,
		&out.Status,
		&out.ExpiresAt,
		&consumedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.UsedAt = consumedAt
	return &out, nil
}
