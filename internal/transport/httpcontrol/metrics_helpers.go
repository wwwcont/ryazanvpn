package httpcontrol

import "time"

func utilizationPercent(currentBPS float64, linkCapacityBPS int64) float64 {
	if linkCapacityBPS <= 0 {
		return 0
	}
	return (currentBPS / float64(linkCapacityBPS)) * 100
}

func isLowUtilizationAnomaly(currentBPS, p95BPS float64, utilPercent float64) bool {
	if p95BPS <= 0 {
		return false
	}
	return utilPercent < 20 && p95BPS >= currentBPS*3
}

func estimateDaysRemaining(balanceKopecks int64, dailyChargeKopecks int64, billableDevices int64) *float64 {
	if balanceKopecks <= 0 || dailyChargeKopecks <= 0 || billableDevices <= 0 {
		return nil
	}
	denom := float64(dailyChargeKopecks * billableDevices)
	if denom <= 0 {
		return nil
	}
	v := float64(balanceKopecks) / denom
	return &v
}

func heartbeatFreshSeconds(lastSeen *time.Time) *int64 {
	if lastSeen == nil {
		return nil
	}
	v := int64(time.Since(lastSeen.UTC()).Seconds())
	if v < 0 {
		v = 0
	}
	return &v
}
