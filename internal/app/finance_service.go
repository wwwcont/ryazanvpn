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

func (s FinanceService) SetBalance(ctx context.Context, userID string, targetBalanceKopecks int64, reference string) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks::bigint FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return err
	}
	delta := targetBalanceKopecks - current
	rawMeta, _ := json.Marshal(map[string]any{"source": "manual_set", "target_balance_kopecks": targetBalanceKopecks})
	if _, err := tx.Exec(ctx, `
UPDATE users
SET balance_kopecks = $2::bigint,
	updated_at = NOW(),
	status = CASE WHEN $2::bigint > 0 THEN 'active' ELSE status END
WHERE id = $1`, userID, targetBalanceKopecks); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3::bigint,$4::bigint,$5,$6::jsonb)`, userID, LedgerManualAdjustment, delta, targetBalanceKopecks, reference, string(rawMeta)); err != nil {
		return err
	}
	resumed, suspended, err := reconcileUserAccessStatusTx(ctx, tx, userID, targetBalanceKopecks)
	if err != nil {
		return err
	}
	slog.Info("finance.balance.set", "user_id", userID, "delta_kopecks", delta, "balance_before_kopecks", current, "balance_after_kopecks", targetBalanceKopecks, "device_accesses_resumed", resumed, "device_accesses_suspended", suspended)
	return tx.Commit(ctx)
}

func (s FinanceService) addLedgerEntry(ctx context.Context, userID string, op string, amountKopecks int64, reference string, metadata map[string]any) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks::bigint FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3::bigint,$4::bigint,$5,$6::jsonb)`, userID, op, amountKopecks, next, reference, string(rawMeta)); err != nil {
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
SELECT u.id::text, COUNT(DISTINCT d.id)::bigint AS billable_devices
FROM users u
JOIN devices d ON d.user_id = u.id AND d.status = 'active'
JOIN device_accesses da ON da.device_id = d.id AND da.status IN ('active', 'suspended_nonpayment')
  AND (u.last_charge_at IS NULL OR u.last_charge_at < CURRENT_DATE)
GROUP BY u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var billableDevices int64
		if err := rows.Scan(&userID, &billableDevices); err != nil {
			return err
		}
		if err := w.chargeUser(ctx, userID, billableDevices); err != nil {
			return fmt.Errorf("charge user %s: %w", userID, err)
		}
	}
	return rows.Err()
}

func (w DailyChargeWorker) chargeUser(ctx context.Context, userID string, billableDevices int64) error {
	if billableDevices <= 0 {
		return nil
	}
	tx, err := w.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	var lastChargeAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks::bigint, last_charge_at FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current, &lastChargeAt); err != nil {
		return err
	}
	if lastChargeAt != nil {
		nowDate := time.Now().UTC().Truncate(24 * time.Hour)
		if lastChargeAt.UTC().Equal(nowDate) || lastChargeAt.UTC().After(nowDate) {
			return tx.Commit(ctx)
		}
	}
	chargeAmount := billableDevices * w.DailyChargeKopecks
	next := current - chargeAmount
	if _, err := tx.Exec(ctx, `UPDATE users SET balance_kopecks = $2::bigint, last_charge_at = CURRENT_DATE, updated_at = NOW() WHERE id = $1`, userID, next); err != nil {
		return err
	}
	rawMeta := fmt.Sprintf(`{"kind":"daily_subscription_charge","billable_devices":%d}`, billableDevices)
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3::bigint,$4::bigint,$5,$6::jsonb)`, userID, LedgerDailyCharge, -chargeAmount, next, "daily_charge", rawMeta); err != nil {
		return err
	}
	resumed, suspended, err := reconcileUserAccessStatusTx(ctx, tx, userID, next)
	if err != nil {
		return err
	}
	slog.Info("finance.daily_charge.applied", "user_id", userID, "delta_kopecks", -chargeAmount, "billable_devices", billableDevices, "balance_before_kopecks", current, "balance_after_kopecks", next, "device_accesses_resumed", resumed, "device_accesses_suspended", suspended)
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
