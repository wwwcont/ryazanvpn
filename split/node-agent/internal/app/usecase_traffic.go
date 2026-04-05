package app

import (
	"context"
	"time"
)

type GetDeviceTrafficTotal struct{ Traffic TrafficRepository }

func (uc GetDeviceTrafficTotal) Execute(ctx context.Context, deviceID string) (int64, error) {
	return uc.Traffic.GetDeviceTrafficTotal(ctx, deviceID)
}

type GetDeviceTrafficLast30Days struct {
	Traffic TrafficRepository
	Now     func() time.Time
}

func (uc GetDeviceTrafficLast30Days) Execute(ctx context.Context, deviceID string) (int64, error) {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}
	return uc.Traffic.GetDeviceTrafficLastNDays(ctx, deviceID, 30, now)
}

type GetUserTrafficTotal struct{ Traffic TrafficRepository }

func (uc GetUserTrafficTotal) Execute(ctx context.Context, userID string) (int64, error) {
	return uc.Traffic.GetUserTrafficTotal(ctx, userID)
}

type GetUserTrafficLast30Days struct {
	Traffic TrafficRepository
	Now     func() time.Time
}

func (uc GetUserTrafficLast30Days) Execute(ctx context.Context, userID string) (int64, error) {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}
	return uc.Traffic.GetUserTrafficLastNDays(ctx, userID, 30, now)
}
