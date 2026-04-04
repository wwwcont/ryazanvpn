package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type execCall struct {
	sql  string
	args []any
}

type fakeTxExec struct {
	calls []execCall
}

func (f *fakeTxExec) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, execCall{sql: sql, args: arguments})
	if strings.Contains(sql, "da.status='suspended_nonpayment'") {
		return pgconn.NewCommandTag("UPDATE 2"), nil
	}
	if strings.Contains(sql, "da.status='active'") {
		return pgconn.NewCommandTag("UPDATE 3"), nil
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func TestManualBalanceAdjustPositive_ReactivatesSuspendedNonpayment(t *testing.T) {
	tx := &fakeTxExec{}
	resumed, suspended, err := reconcileUserAccessStatusTx(context.Background(), tx, "user-1", 100)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resumed != 2 || suspended != 0 {
		t.Fatalf("unexpected rows affected: resumed=%d suspended=%d", resumed, suspended)
	}
	if len(tx.calls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(tx.calls))
	}
	if len(tx.calls[1].args) != 1 || tx.calls[1].args[0] != "user-1" {
		t.Fatalf("expected user_id argument for access update, got %#v", tx.calls[1].args)
	}
}

func TestManualBalanceAdjustToZero_SuspendsActiveAccesses(t *testing.T) {
	tx := &fakeTxExec{}
	resumed, suspended, err := reconcileUserAccessStatusTx(context.Background(), tx, "user-2", 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resumed != 0 || suspended != 3 {
		t.Fatalf("unexpected rows affected: resumed=%d suspended=%d", resumed, suspended)
	}
	if len(tx.calls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(tx.calls))
	}
	if len(tx.calls[1].args) != 1 || tx.calls[1].args[0] != "user-2" {
		t.Fatalf("expected user_id argument for access update, got %#v", tx.calls[1].args)
	}
}

func TestTopUpFromSuspendedNonpayment_ReactivatesAccesses(t *testing.T) {
	tx := &fakeTxExec{}
	resumed, suspended, err := reconcileUserAccessStatusTx(context.Background(), tx, "user-3", 25_000)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resumed == 0 {
		t.Fatalf("expected reactivated accesses, got resumed=%d", resumed)
	}
	if suspended != 0 {
		t.Fatalf("did not expect suspended rows, got %d", suspended)
	}
}
