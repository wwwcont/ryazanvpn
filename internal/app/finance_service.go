package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	LedgerInviteBonus      = "invite_bonus"
	LedgerDailyCharge      = "daily_charge"
	LedgerPayment          = "payment"
	LedgerManualAdjustment = "manual_adjustment"
)

type FinanceService struct {
	PG *pgxpool.Pool
}

func (s FinanceService) ApplyInviteBonus(ctx context.Context, userID string, inviteCodeID string, amountKopecks int64) error {
	return s.addLedgerEntry(ctx, userID, LedgerInviteBonus, amountKopecks, inviteCodeID, map[string]any{"invite_code_id": inviteCodeID})
}

func (s FinanceService) AddPayment(ctx context.Context, userID string, amountKopecks int64, reference string) error {
	return s.addLedgerEntry(ctx, userID, LedgerPayment, amountKopecks, reference, map[string]any{"source": "payment"})
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
	if _, err := tx.Exec(ctx, `UPDATE users SET balance_kopecks = $2, updated_at = NOW(), status = CASE WHEN $2 > 0 THEN 'active' ELSE status END WHERE id = $1`, userID, next); err != nil {
		return err
	}
	rawMeta, _ := json.Marshal(metadata)
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`, userID, op, amountKopecks, next, reference, string(rawMeta)); err != nil {
		return err
	}
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
		w.DailyChargeKopecks = 800
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
JOIN access_grants ag ON ag.user_id = u.id
WHERE ag.status = 'active'
  AND ag.expires_at > NOW()
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
	if err := tx.QueryRow(ctx, `SELECT balance_kopecks FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return err
	}
	next := current - w.DailyChargeKopecks
	if _, err := tx.Exec(ctx, `UPDATE users SET balance_kopecks = $2, last_charge_at = CURRENT_DATE, status = CASE WHEN $2 <= 0 THEN 'blocked' ELSE status END, updated_at = NOW() WHERE id = $1`, userID, next); err != nil {
		return err
	}
	rawMeta := `{"kind":"daily_subscription_charge"}`
	if _, err := tx.Exec(ctx, `INSERT INTO user_ledger (user_id, operation_type, amount_kopecks, balance_after_kopecks, reference, metadata) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`, userID, LedgerDailyCharge, -w.DailyChargeKopecks, next, "daily_charge", rawMeta); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if next <= 0 {
		accessRows, err := w.PG.Query(ctx, `SELECT da.id::text FROM device_accesses da JOIN devices d ON d.id = da.device_id WHERE d.user_id = $1 AND da.status = 'active'`, userID)
		if err != nil {
			return err
		}
		defer accessRows.Close()
		for accessRows.Next() {
			var accessID string
			if err := accessRows.Scan(&accessID); err != nil {
				return err
			}
			if err := w.RevokeAccess.Execute(ctx, RevokeDeviceAccessInput{AccessID: accessID}); err != nil {
				return err
			}
		}
	}
	return nil
}
