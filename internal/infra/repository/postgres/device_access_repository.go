package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/access"
)

type DeviceAccessRepository struct {
	q dbtx
}

func NewDeviceAccessRepository(pool *pgxpool.Pool) *DeviceAccessRepository {
	return &DeviceAccessRepository{q: pool}
}

func (r *DeviceAccessRepository) WithTx(tx pgx.Tx) *DeviceAccessRepository {
	return &DeviceAccessRepository{q: tx}
}

func (r *DeviceAccessRepository) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	const query = `
INSERT INTO device_accesses (device_id, vpn_node_id, protocol, status, assigned_ip, preshared_key)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, device_id::text, vpn_node_id::text, protocol, status, assigned_ip::text, preshared_key, config_blob_encrypted, granted_at, revoked_at, created_at, updated_at`

	out, err := scanAccess(r.q.QueryRow(ctx, query, in.DeviceID, in.VPNNodeID, in.Protocol, in.Status, in.AssignedIP, in.PresharedKey))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DeviceAccessRepository) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	const query = `
SELECT id::text, device_id::text, vpn_node_id::text, protocol, status, assigned_ip::text, preshared_key, config_blob_encrypted, granted_at, revoked_at, created_at, updated_at
FROM device_accesses
WHERE id = $1`

	out, err := scanAccess(r.q.QueryRow(ctx, query, id))
	if isNoRows(err) {
		return nil, access.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DeviceAccessRepository) Activate(ctx context.Context, id string, grantedAt time.Time) error {
	const query = `
UPDATE device_accesses
SET status = 'active', granted_at = $2, revoked_at = NULL, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, grantedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return access.ErrNotFound
	}
	return nil
}

func (r *DeviceAccessRepository) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	const query = `
UPDATE device_accesses
SET status = 'revoked', revoked_at = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, revokedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return access.ErrNotFound
	}
	return nil
}

func (r *DeviceAccessRepository) MarkError(ctx context.Context, id string, failedAt time.Time, _ string) error {
	const query = `
UPDATE device_accesses
SET status = 'error', revoked_at = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, failedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return access.ErrNotFound
	}
	return nil
}

func (r *DeviceAccessRepository) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	const query = `
UPDATE device_accesses
SET config_blob_encrypted = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, blob)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return access.ErrNotFound
	}
	return nil
}

func (r *DeviceAccessRepository) ClearConfigBlobEncrypted(ctx context.Context, id string) error {
	const query = `
UPDATE device_accesses
SET config_blob_encrypted = NULL, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return access.ErrNotFound
	}
	return nil
}

func (r *DeviceAccessRepository) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	const query = `
SELECT id::text, device_id::text, vpn_node_id::text, protocol, status, assigned_ip::text, preshared_key, config_blob_encrypted, granted_at, revoked_at, created_at, updated_at
FROM device_accesses
WHERE device_id = $1 AND status = 'active'
ORDER BY created_at DESC`

	rows, err := r.q.Query(ctx, query, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*access.DeviceAccess
	for rows.Next() {
		item, err := scanAccess(rows)
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

func (r *DeviceAccessRepository) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	const query = `
SELECT id::text, device_id::text, vpn_node_id::text, protocol, status, assigned_ip::text, preshared_key, config_blob_encrypted, granted_at, revoked_at, created_at, updated_at
FROM device_accesses
WHERE vpn_node_id = $1 AND status = 'active' AND assigned_ip = $2
ORDER BY created_at DESC
LIMIT 1`

	out, err := scanAccess(r.q.QueryRow(ctx, query, nodeID, assignedIP))
	if isNoRows(err) {
		return nil, access.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DeviceAccessRepository) ListActiveByNodeID(ctx context.Context, nodeID string) ([]*access.DeviceAccess, error) {
	const query = `
SELECT id::text, device_id::text, vpn_node_id::text, protocol, status, assigned_ip::text, preshared_key, config_blob_encrypted, granted_at, revoked_at, created_at, updated_at
FROM device_accesses
WHERE vpn_node_id = $1 AND status = 'active'
ORDER BY created_at DESC`

	rows, err := r.q.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*access.DeviceAccess
	for rows.Next() {
		item, err := scanAccess(rows)
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

func scanAccess(row pgx.Row) (*access.DeviceAccess, error) {
	var out access.DeviceAccess
	var grantedAt *time.Time
	var revokedAt *time.Time
	err := row.Scan(
		&out.ID,
		&out.DeviceID,
		&out.VPNNodeID,
		&out.Protocol,
		&out.Status,
		&out.AssignedIP,
		&out.PresharedKey,
		&out.ConfigBlobEncrypted,
		&grantedAt,
		&revokedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.GrantedAt = grantedAt
	out.RevokedAt = revokedAt
	return &out, nil
}
