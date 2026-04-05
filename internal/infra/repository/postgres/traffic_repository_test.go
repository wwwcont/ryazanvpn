package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
)

type trFakeDB struct {
	sqls []string
}

func (f *trFakeDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	f.sqls = append(f.sqls, sql)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (f *trFakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}
func (f *trFakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func TestAddDailyUsageDelta_UsesBigintCasts(t *testing.T) {
	fake := &trFakeDB{}
	repo := &TrafficRepository{q: fake}
	err := repo.AddDailyUsageDelta(context.Background(), traffic.AddDailyUsageDeltaParams{
		DeviceID:  "d1",
		Protocol:  "wireguard",
		UsageDate: time.Now().UTC(),
		RXDelta:   10,
		TXDelta:   20,
	})
	if err != nil {
		t.Fatalf("AddDailyUsageDelta failed: %v", err)
	}
	if len(fake.sqls) < 3 {
		t.Fatalf("expected 3 sql statements, got %d", len(fake.sqls))
	}
	for _, stmt := range fake.sqls {
		if !strings.Contains(stmt, "::bigint") {
			t.Fatalf("expected explicit bigint casts, got query: %s", stmt)
		}
	}
}

func TestAddNodeThroughputSample_UsesInsertAndBps(t *testing.T) {
	fake := &trFakeDB{}
	repo := &TrafficRepository{q: fake}
	err := repo.AddNodeThroughputSample(context.Background(), traffic.AddNodeThroughputSampleParams{
		NodeID:        "n1",
		CapturedAt:    time.Now().UTC(),
		WindowSec:     60,
		RXDelta:       600,
		TXDelta:       300,
		PeerCount:     10,
		ResolvedPeers: 8,
	})
	if err != nil {
		t.Fatalf("AddNodeThroughputSample failed: %v", err)
	}
	if len(fake.sqls) == 0 || !strings.Contains(fake.sqls[0], "INSERT INTO node_throughput_samples") {
		t.Fatalf("expected throughput insert query, got: %+v", fake.sqls)
	}
}

func TestCleanupNodeThroughputSamples_UsesDeleteByCapturedAt(t *testing.T) {
	fake := &trFakeDB{}
	repo := &TrafficRepository{q: fake}
	err := repo.CleanupNodeThroughputSamples(context.Background(), time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupNodeThroughputSamples failed: %v", err)
	}
	if len(fake.sqls) == 0 || !strings.Contains(fake.sqls[0], "DELETE FROM node_throughput_samples") {
		t.Fatalf("expected cleanup delete query, got: %+v", fake.sqls)
	}
}
