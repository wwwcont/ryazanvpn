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
