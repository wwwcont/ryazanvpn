package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	LedgerInviteBonus      = "invite_bonus"
	LedgerDailyCharge      = "daily_charge"
	LedgerTopup            = "topup"
	LedgerManualAdjustment = "manual_adjustment"
)

type FinanceService struct {
	PG *pgxpool.Pool
}

func (s FinanceService) ApplyInviteBonus(ctx context.Context, userID string, inviteCodeID string, amountKopecks int64) error {
	return s.addLedgerEntry(ctx, userID, LedgerInviteBonus, amountKopecks, inviteCodeID, map[string]any{"invite_code_id": inviteCodeID})
}

func (s FinanceService) AddPayment(ctx context.Context, userID string, amountKopecks int64, reference string) error {
	return s.addLedgerEntry(ctx, userID, LedgerTopup, amountKopecks, reference, map[string]any{"source": "payment"})
}

func (s FinanceService) AddManualAdjustment(ctx context.Context, userID string, amountKopecks int64, reference string) error {
	return s.addLedgerEntry(ctx, userID, LedgerManualAdjustment, amountKopecks, reference, map[string]any{"source": "manual"})
}

func (s FinanceService) addLedgerEntry(ctx context.Context, userID string, op string, amountKopecks int64, reference string, metadata map[string]any) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return err
	}
	next := current + amountKopecks
	if err := tx.QueryRow(ctx, `
UPDATE users
SET balance_kopecks = $2::bigint,
	updated_at = NOW(),
	status = CASE WHEN $2::bigint > 0 THEN 'active' ELSE status END
WHERE id = $1
RETURNING balance_kopecks`, userID, next).Scan(&next); err != nil {
		return err
	}
	rawMeta, _ := json.Marshal(metadata)
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`, userID, op, amountKopecks, next, reference, string(rawMeta)); err != nil {
		return err
	}
	resumed, suspended, err := reconcileUserAccessStatusTx(ctx, tx, userID, next)
	if err != nil {
		return err
	}
	slog.Info("finance.balance.adjusted", "user_id", userID, "operation_type", op, "delta_kopecks", amountKopecks, "balance_before_kopecks", current, "balance_after_kopecks", next, "device_accesses_resumed", resumed, "device_accesses_suspended", suspended)
	return tx.Commit(ctx)
}

type DailyChargeWorker struct {
	PG                 *pgxpool.Pool
	RevokeAccess       RevokeDeviceAccess
	Interval           time.Duration
	DailyChargeKopecks int64
}

func (w DailyChargeWorker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	if w.DailyChargeKopecks <= 0 {
		w.DailyChargeKopecks = 1000
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	_ = w.charge(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = w.charge(ctx)
		}
	}
}

func (w DailyChargeWorker) charge(ctx context.Context) error {
	rows, err := w.PG.Query(ctx, `
SELECT u.id::text
FROM users u
WHERE EXISTS (
	SELECT 1
	FROM devices d
	JOIN device_accesses da ON da.device_id = d.id
	WHERE d.user_id = u.id
	  AND d.status = 'active'
	  AND da.status IN ('active', 'suspended_nonpayment')
)
  AND (u.last_charge_at IS NULL OR u.last_charge_at < CURRENT_DATE)
GROUP BY u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return err
		}
		if err := w.chargeUser(ctx, userID); err != nil {
			return fmt.Errorf("charge user %s: %w", userID, err)
		}
	}
	return rows.Err()
}

func (w DailyChargeWorker) chargeUser(ctx context.Context, userID string) error {
	tx, err := w.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	var lastChargeAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks, last_charge_at FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current, &lastChargeAt); err != nil {
		return err
	}
	if lastChargeAt != nil {
		nowDate := time.Now().UTC().Truncate(24 * time.Hour)
		if lastChargeAt.UTC().Equal(nowDate) || lastChargeAt.UTC().After(nowDate) {
			return tx.Commit(ctx)
		}
	}
	next := current - w.DailyChargeKopecks
	if _, err := tx.Exec(ctx, `UPDATE users SET balance_kopecks = $2, last_charge_at = CURRENT_DATE, updated_at = NOW() WHERE id = $1`, userID, next); err != nil {
		return err
	}
	rawMeta := `{"kind":"daily_subscription_charge"}`
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`, userID, LedgerDailyCharge, -w.DailyChargeKopecks, next, "daily_charge", rawMeta); err != nil {
		return err
	}
	resumed, suspended, err := reconcileUserAccessStatusTx(ctx, tx, userID, next)
	if err != nil {
		return err
	}
	slog.Info("finance.daily_charge.applied", "user_id", userID, "delta_kopecks", -w.DailyChargeKopecks, "balance_before_kopecks", current, "balance_after_kopecks", next, "device_accesses_resumed", resumed, "device_accesses_suspended", suspended)
	return tx.Commit(ctx)
}

type txExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func reconcileUserAccessStatusTx(ctx context.Context, tx txExec, userID string, balance int64) (resumed int64, suspended int64, err error) {
	if balance > 0 {
		if _, err := tx.Exec(ctx, `UPDATE users SET status='active', updated_at=NOW() WHERE id=$1`, userID); err != nil {
			return 0, 0, err
		}
		tag, err := tx.Exec(ctx, `
UPDATE device_accesses da
SET status='active', updated_at=NOW()
FROM devices d
WHERE da.device_id=d.id
  AND d.user_id=$1
  AND da.status='suspended_nonpayment'`, userID)
		return tag.RowsAffected(), 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET status='blocked_for_nonpayment', updated_at=NOW() WHERE id=$1`, userID); err != nil {
		return 0, 0, err
	}
	tag, err := tx.Exec(ctx, `
UPDATE device_accesses da
SET status='suspended_nonpayment', updated_at=NOW()
FROM devices d
WHERE da.device_id=d.id
  AND d.user_id=$1
  AND da.status='active'`, userID)
	return 0, tag.RowsAffected(), err
}
