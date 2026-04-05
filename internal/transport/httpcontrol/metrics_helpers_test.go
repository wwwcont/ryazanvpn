package httpcontrol

import (
	"testing"
	"time"
)

func TestUtilizationPercent(t *testing.T) {
	got := utilizationPercent(100, 1000)
	if got != 10 {
		t.Fatalf("expected 10, got %v", got)
	}
}

func TestIsLowUtilizationAnomaly(t *testing.T) {
	if !isLowUtilizationAnomaly(10, 40, 5) {
		t.Fatal("expected anomaly")
	}
	if isLowUtilizationAnomaly(10, 20, 5) {
		t.Fatal("did not expect anomaly")
	}
}

func TestEstimateDaysRemaining(t *testing.T) {
	got := estimateDaysRemaining(10_000, 1_000, 2)
	if got == nil {
		t.Fatal("expected value")
	}
	if *got != 5 {
		t.Fatalf("expected 5, got %v", *got)
	}
}

func TestHeartbeatFreshSeconds(t *testing.T) {
	now := time.Now().UTC().Add(-30 * time.Second)
	got := heartbeatFreshSeconds(&now)
	if got == nil || *got < 1 {
		t.Fatalf("expected positive freshness, got %v", got)
	}
}
