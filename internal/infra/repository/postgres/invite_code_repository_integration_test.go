package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInviteCodeRepository_UsageFlow(t *testing.T) {
	dsn := os.Getenv("INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("INTEGRATION_DB_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	var userID string
	err = pool.QueryRow(ctx, `
INSERT INTO users (telegram_id, username, status)
VALUES (999999001, 'integration-user', 'active')
ON CONFLICT (telegram_id) DO UPDATE SET updated_at = NOW()
RETURNING id::text`).Scan(&userID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewInviteCodeRepository(pool)
	created, err := repo.Create(ctx, invitecode.CreateParams{Code: "ITEST-1", Status: invitecode.CodeStatusActive, MaxActivations: 1})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := repo.WithTx(tx)
	locked, err := txRepo.GetByCodeForUpdate(ctx, created.Code)
	if err != nil {
		t.Fatalf("get for update: %v", err)
	}
	if locked.CurrentActivations != 0 {
		t.Fatalf("expected current_activations=0, got %d", locked.CurrentActivations)
	}

	if _, err := txRepo.CreateActivation(ctx, invitecode.CreateActivationParams{InviteCodeID: locked.ID, UserID: userID}); err != nil {
		t.Fatalf("create activation: %v", err)
	}

	updated, err := txRepo.IncrementUsageAndMaybeExhaust(ctx, locked.ID)
	if err != nil {
		t.Fatalf("increment usage: %v", err)
	}
	if updated.Status != invitecode.CodeStatusExhausted {
		t.Fatalf("expected exhausted status, got %s", updated.Status)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
}
