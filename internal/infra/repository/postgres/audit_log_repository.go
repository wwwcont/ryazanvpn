package postgres

import (
	"context"

	"github.com/example/ryazanvpn/internal/domain/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogRepository struct {
	q dbtx
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{q: pool}
}

func (r *AuditLogRepository) WithTx(tx pgx.Tx) *AuditLogRepository {
	return &AuditLogRepository{q: tx}
}

func (r *AuditLogRepository) Create(ctx context.Context, in audit.CreateParams) (*audit.Log, error) {
	const query = `
INSERT INTO audit_logs (actor_user_id, entity_type, entity_id, action, details)
VALUES ($1, $2, $3, $4, $5::jsonb)
RETURNING id::text, actor_user_id::text, entity_type, entity_id::text, action, details::text, created_at, updated_at`

	var out audit.Log
	var actorUserID *string
	var entityID *string
	err := r.q.QueryRow(ctx, query, in.ActorUserID, in.EntityType, in.EntityID, in.Action, in.DetailsJSON).Scan(
		&out.ID,
		&actorUserID,
		&out.EntityType,
		&entityID,
		&out.Action,
		&out.DetailsJSON,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.ActorUserID = actorUserID
	out.EntityID = entityID
	return &out, nil
}
