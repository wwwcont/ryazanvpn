package postgres

import (
	"context"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeviceRepository struct {
	q dbtx
}

func NewDeviceRepository(pool *pgxpool.Pool) *DeviceRepository {
	return &DeviceRepository{q: pool}
}

func (r *DeviceRepository) WithTx(tx pgx.Tx) *DeviceRepository {
	return &DeviceRepository{q: tx}
}

func (r *DeviceRepository) List(ctx context.Context) ([]*device.Device, error) {
	const query = `
SELECT id::text, user_id::text, vpn_node_id::text, public_key, COALESCE(name, ''), COALESCE(platform, ''), status, last_seen_at, created_at, updated_at
FROM devices
ORDER BY created_at DESC`

	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*device.Device
	for rows.Next() {
		item, err := scanDevice(rows)
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

func (r *DeviceRepository) Create(ctx context.Context, in device.CreateParams) (*device.Device, error) {
	const query = `
INSERT INTO devices (user_id, vpn_node_id, public_key, name, platform, status)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
RETURNING id::text, user_id::text, vpn_node_id::text, public_key, COALESCE(name, ''), COALESCE(platform, ''), status, last_seen_at, created_at, updated_at`

	out, err := scanDevice(r.q.QueryRow(ctx, query, in.UserID, in.VPNNodeID, in.PublicKey, in.Name, in.Platform, in.Status))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DeviceRepository) ListByUserID(ctx context.Context, userID string) ([]*device.Device, error) {
	const query = `
SELECT id::text, user_id::text, vpn_node_id::text, public_key, COALESCE(name, ''), COALESCE(platform, ''), status, last_seen_at, created_at, updated_at
FROM devices
WHERE user_id = $1
ORDER BY created_at DESC`

	rows, err := r.q.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*device.Device
	for rows.Next() {
		item, err := scanDevice(rows)
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

func (r *DeviceRepository) GetActiveByUserID(ctx context.Context, userID string) (*device.Device, error) {
	const query = `
SELECT id::text, user_id::text, vpn_node_id::text, public_key, COALESCE(name, ''), COALESCE(platform, ''), status, last_seen_at, created_at, updated_at
FROM devices
WHERE user_id = $1 AND status = 'active'
ORDER BY created_at DESC
LIMIT 1`

	out, err := scanDevice(r.q.QueryRow(ctx, query, userID))
	if isNoRows(err) {
		return nil, device.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DeviceRepository) Revoke(ctx context.Context, id string) error {
	const query = `
UPDATE devices
SET status = 'revoked', updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return device.ErrNotFound
	}
	return nil
}

func (r *DeviceRepository) AssignNode(ctx context.Context, id string, vpnNodeID string) error {
	const query = `
UPDATE devices
SET vpn_node_id = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, vpnNodeID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return device.ErrNotFound
	}
	return nil
}
func scanDevice(row pgx.Row) (*device.Device, error) {
	var out device.Device
	var nodeID *string
	var lastSeen *time.Time
	err := row.Scan(
		&out.ID,
		&out.UserID,
		&nodeID,
		&out.PublicKey,
		&out.Name,
		&out.Platform,
		&out.Status,
		&lastSeen,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.VPNNodeID = nodeID
	out.LastSeenAt = lastSeen
	return &out, nil
}
