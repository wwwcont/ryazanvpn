package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
)

type TrafficRepository struct{ q dbtx }

func NewTrafficRepository(pool *pgxpool.Pool) *TrafficRepository { return &TrafficRepository{q: pool} }

func (r *TrafficRepository) CreateSnapshot(ctx context.Context, in traffic.CreateSnapshotParams) (*traffic.DeviceTrafficSnapshot, error) {
	const query = `
INSERT INTO device_traffic_snapshots (device_id, captured_at, rx_total_bytes, tx_total_bytes)
VALUES ($1, $2, $3, $4)
RETURNING id::text, device_id::text, captured_at, rx_total_bytes, tx_total_bytes, created_at`
	var out traffic.DeviceTrafficSnapshot
	err := r.q.QueryRow(ctx, query, in.DeviceID, in.CapturedAt, in.RXTotalBytes, in.TXTotalBytes).
		Scan(&out.ID, &out.DeviceID, &out.CapturedAt, &out.RXTotalBytes, &out.TXTotalBytes, &out.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *TrafficRepository) GetLastSnapshotByDeviceID(ctx context.Context, deviceID string, before time.Time) (*traffic.DeviceTrafficSnapshot, error) {
	const query = `
SELECT id::text, device_id::text, captured_at, rx_total_bytes, tx_total_bytes, created_at
FROM device_traffic_snapshots
WHERE device_id = $1 AND captured_at < $2
ORDER BY captured_at DESC
LIMIT 1`
	var out traffic.DeviceTrafficSnapshot
	err := r.q.QueryRow(ctx, query, deviceID, before).Scan(&out.ID, &out.DeviceID, &out.CapturedAt, &out.RXTotalBytes, &out.TXTotalBytes, &out.CreatedAt)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *TrafficRepository) AddDailyUsageDelta(ctx context.Context, in traffic.AddDailyUsageDeltaParams) error {
	const query = `
INSERT INTO traffic_usage_daily (device_id, usage_date, rx_bytes, tx_bytes, total_bytes)
VALUES ($1, $2::date, $3, $4, $3+$4)
ON CONFLICT (device_id, usage_date)
DO UPDATE SET
  rx_bytes = traffic_usage_daily.rx_bytes + EXCLUDED.rx_bytes,
  tx_bytes = traffic_usage_daily.tx_bytes + EXCLUDED.tx_bytes,
  total_bytes = traffic_usage_daily.total_bytes + EXCLUDED.total_bytes,
  updated_at = NOW()`
	_, err := r.q.Exec(ctx, query, in.DeviceID, in.UsageDate, max64(in.RXDelta, 0), max64(in.TXDelta, 0))
	return err
}

func (r *TrafficRepository) GetDeviceTrafficTotal(ctx context.Context, deviceID string) (int64, error) {
	const query = `SELECT COALESCE(SUM(total_bytes),0) FROM traffic_usage_daily WHERE device_id = $1`
	var out int64
	err := r.q.QueryRow(ctx, query, deviceID).Scan(&out)
	return out, err
}

func (r *TrafficRepository) GetDeviceTrafficLastNDays(ctx context.Context, deviceID string, days int, now time.Time) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	from := now.UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	const query = `SELECT COALESCE(SUM(total_bytes),0) FROM traffic_usage_daily WHERE device_id = $1 AND usage_date >= $2::date`
	var out int64
	err := r.q.QueryRow(ctx, query, deviceID, from).Scan(&out)
	return out, err
}

func (r *TrafficRepository) GetUserTrafficTotal(ctx context.Context, userID string) (int64, error) {
	const query = `
SELECT COALESCE(SUM(tud.total_bytes),0)
FROM traffic_usage_daily tud
JOIN devices d ON d.id = tud.device_id
WHERE d.user_id = $1`
	var out int64
	err := r.q.QueryRow(ctx, query, userID).Scan(&out)
	return out, err
}

func (r *TrafficRepository) GetUserTrafficLastNDays(ctx context.Context, userID string, days int, now time.Time) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	from := now.UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	const query = `
SELECT COALESCE(SUM(tud.total_bytes),0)
FROM traffic_usage_daily tud
JOIN devices d ON d.id = tud.device_id
WHERE d.user_id = $1 AND tud.usage_date >= $2::date`
	var out int64
	err := r.q.QueryRow(ctx, query, userID, from).Scan(&out)
	return out, err
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ pgx.Row
