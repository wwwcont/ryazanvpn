package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

type UserRepository struct {
	q dbtx
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{q: pool}
}

func (r *UserRepository) WithTx(tx pgx.Tx) *UserRepository {
	return &UserRepository{q: tx}
}

func (r *UserRepository) List(ctx context.Context) ([]*user.User, error) {
	const query = `
SELECT id::text, telegram_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(last_name, ''), status, balance_kopecks, last_charge_at, created_at, updated_at
FROM users
ORDER BY created_at DESC`

	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*user.User
	for rows.Next() {
		item := &user.User{}
		if err := rows.Scan(
			&item.ID,
			&item.TelegramID,
			&item.Username,
			&item.FirstName,
			&item.LastName,
			&item.Status,
			&item.BalanceKopecks,
			&item.LastChargeAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*user.User, error) {
	const query = `
SELECT id::text, telegram_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(last_name, ''), status, balance_kopecks, last_charge_at, created_at, updated_at
FROM users
WHERE telegram_id = $1`

	var out user.User
	err := r.q.QueryRow(ctx, query, telegramID).Scan(
		&out.ID,
		&out.TelegramID,
		&out.Username,
		&out.FirstName,
		&out.LastName,
		&out.Status,
		&out.BalanceKopecks,
		&out.LastChargeAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	const query = `
SELECT id::text, telegram_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(last_name, ''), status, balance_kopecks, last_charge_at, created_at, updated_at
FROM users
WHERE LOWER(username) = LOWER($1)
LIMIT 1`

	var out user.User
	err := r.q.QueryRow(ctx, query, username).Scan(
		&out.ID,
		&out.TelegramID,
		&out.Username,
		&out.FirstName,
		&out.LastName,
		&out.Status,
		&out.BalanceKopecks,
		&out.LastChargeAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *UserRepository) Create(ctx context.Context, in user.CreateParams) (*user.User, error) {
	const query = `
INSERT INTO users (telegram_id, username, first_name, last_name, status)
VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5)
RETURNING id::text, telegram_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(last_name, ''), status, balance_kopecks, last_charge_at, created_at, updated_at`

	var out user.User
	err := r.q.QueryRow(ctx, query, in.TelegramID, in.Username, in.FirstName, in.LastName, in.Status).Scan(
		&out.ID,
		&out.TelegramID,
		&out.Username,
		&out.FirstName,
		&out.LastName,
		&out.Status,
		&out.BalanceKopecks,
		&out.LastChargeAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	const query = `
SELECT id::text, telegram_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(last_name, ''), status, balance_kopecks, last_charge_at, created_at, updated_at
FROM users
WHERE id = $1`

	var out user.User
	err := r.q.QueryRow(ctx, query, id).Scan(
		&out.ID,
		&out.TelegramID,
		&out.Username,
		&out.FirstName,
		&out.LastName,
		&out.Status,
		&out.BalanceKopecks,
		&out.LastChargeAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if isNoRows(err) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}
