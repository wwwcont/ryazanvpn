package postgres

import (
	"context"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NodeOperationRepository struct {
	q dbtx
}

func NewNodeOperationRepository(pool *pgxpool.Pool) *NodeOperationRepository {
	return &NodeOperationRepository{q: pool}
}

func (r *NodeOperationRepository) WithTx(tx pgx.Tx) *NodeOperationRepository {
	return &NodeOperationRepository{q: tx}
}

func (r *NodeOperationRepository) GetByID(ctx context.Context, id string) (*operation.NodeOperation, error) {
	const query = `
SELECT id::text, vpn_node_id::text, operation_type, status, requested_by_user_id::text,
       payload::text, error_message, started_at, finished_at, created_at, updated_at
FROM node_operations
WHERE id = $1`

	out, err := scanNodeOperation(r.q.QueryRow(ctx, query, id))
	if isNoRows(err) {
		return nil, operation.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *NodeOperationRepository) Create(ctx context.Context, in operation.CreateParams) (*operation.NodeOperation, error) {
	const query = `
INSERT INTO node_operations (vpn_node_id, operation_type, status, requested_by_user_id, payload)
VALUES ($1, $2, $3, $4, $5::jsonb)
RETURNING id::text, vpn_node_id::text, operation_type, status, requested_by_user_id::text,
          payload::text, error_message, started_at, finished_at, created_at, updated_at`

	out, err := scanNodeOperation(r.q.QueryRow(ctx, query, in.VPNNodeID, in.OperationType, in.Status, in.RequestedByUserID, in.PayloadJSON))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *NodeOperationRepository) MarkRunning(ctx context.Context, id string, startedAt time.Time) error {
	const query = `
UPDATE node_operations
SET status = 'running', started_at = $2, updated_at = NOW()
WHERE id = $1`
	return r.execStatusChange(ctx, query, id, startedAt)
}

func (r *NodeOperationRepository) MarkSuccess(ctx context.Context, id string, finishedAt time.Time) error {
	const query = `
UPDATE node_operations
SET status = 'succeeded', finished_at = $2, error_message = NULL, updated_at = NOW()
WHERE id = $1`
	return r.execStatusChange(ctx, query, id, finishedAt)
}

func (r *NodeOperationRepository) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) error {
	const query = `
UPDATE node_operations
SET status = 'failed', finished_at = $2, error_message = $3, updated_at = NOW()
WHERE id = $1`
	res, err := r.q.Exec(ctx, query, id, finishedAt, message)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return operation.ErrNotFound
	}
	return nil
}

func (r *NodeOperationRepository) execStatusChange(ctx context.Context, query string, id string, at time.Time) error {
	res, err := r.q.Exec(ctx, query, id, at)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return operation.ErrNotFound
	}
	return nil
}

func scanNodeOperation(row pgx.Row) (*operation.NodeOperation, error) {
	var out operation.NodeOperation
	var userID *string
	var errMsg *string
	var startedAt *time.Time
	var finishedAt *time.Time
	err := row.Scan(
		&out.ID,
		&out.VPNNodeID,
		&out.OperationType,
		&out.Status,
		&userID,
		&out.PayloadJSON,
		&errMsg,
		&startedAt,
		&finishedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.RequestedByUserID = userID
	out.ErrorMessage = errMsg
	out.StartedAt = startedAt
	out.FinishedAt = finishedAt
	return &out, nil
}
