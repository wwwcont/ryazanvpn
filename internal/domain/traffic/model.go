package traffic

import (
	"context"
	"time"
)

type DeviceTrafficSnapshot struct {
	ID           string
	DeviceID     string
	CapturedAt   time.Time
	RXTotalBytes int64
	TXTotalBytes int64
	CreatedAt    time.Time
}

type TrafficUsageDaily struct {
	ID         string
	DeviceID   string
	UsageDate  time.Time
	RXBytes    int64
	TXBytes    int64
	TotalBytes int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Repository interface {
	CreateSnapshot(ctx context.Context, in CreateSnapshotParams) (*DeviceTrafficSnapshot, error)
	GetLastSnapshotByDeviceID(ctx context.Context, deviceID string, before time.Time) (*DeviceTrafficSnapshot, error)
	AddDailyUsageDelta(ctx context.Context, in AddDailyUsageDeltaParams) error
	GetDeviceTrafficTotal(ctx context.Context, deviceID string) (int64, error)
	GetDeviceTrafficLastNDays(ctx context.Context, deviceID string, days int, now time.Time) (int64, error)
	GetUserTrafficTotal(ctx context.Context, userID string) (int64, error)
	GetUserTrafficLastNDays(ctx context.Context, userID string, days int, now time.Time) (int64, error)
}

type CreateSnapshotParams struct {
	DeviceID     string
	CapturedAt   time.Time
	RXTotalBytes int64
	TXTotalBytes int64
}

type AddDailyUsageDeltaParams struct {
	DeviceID  string
	UsageDate time.Time
	RXDelta   int64
	TXDelta   int64
}
