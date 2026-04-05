package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

type NodeRepository struct {
	q dbtx
}

func NewNodeRepository(pool *pgxpool.Pool) *NodeRepository {
	return &NodeRepository{q: pool}
}

func (r *NodeRepository) WithTx(tx pgx.Tx) *NodeRepository {
	return &NodeRepository{q: tx}
}

func (r *NodeRepository) ListAll(ctx context.Context) ([]*node.Node, error) {
	const query = `
SELECT id::text, name, region, agent_base_url, vpn_endpoint, vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr, runtime_metadata, status, current_load, user_capacity, last_seen_at, created_at, updated_at
FROM vpn_nodes
ORDER BY name`

	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*node.Node
	for rows.Next() {
		item, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *NodeRepository) ListActive(ctx context.Context) ([]*node.Node, error) {
	const query = `
SELECT id::text, name, region, agent_base_url, vpn_endpoint, vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr, runtime_metadata, status, current_load, user_capacity, last_seen_at, created_at, updated_at
FROM vpn_nodes
WHERE status = 'active'
ORDER BY name`

	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*node.Node
	for rows.Next() {
		item, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *NodeRepository) GetByID(ctx context.Context, id string) (*node.Node, error) {
	const query = `
SELECT id::text, name, region, agent_base_url, vpn_endpoint, vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr, runtime_metadata, status, current_load, user_capacity, last_seen_at, created_at, updated_at
FROM vpn_nodes
WHERE id = $1`

	out, err := scanNode(r.q.QueryRow(ctx, query, id))
	if isNoRows(err) {
		return nil, node.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *NodeRepository) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	const query = `
UPDATE vpn_nodes
SET status = $2, last_seen_at = $3, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, status, lastSeenAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return node.ErrNotFound
	}
	return nil
}

func (r *NodeRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	const query = `
UPDATE vpn_nodes
SET status = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, status)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return node.ErrNotFound
	}
	return nil
}

func (r *NodeRepository) UpdateLoad(ctx context.Context, id string, currentLoad int) error {
	const query = `
UPDATE vpn_nodes
SET current_load = $2, updated_at = NOW()
WHERE id = $1`

	res, err := r.q.Exec(ctx, query, id, currentLoad)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return node.ErrNotFound
	}
	return nil
}

func scanNode(row pgx.Row) (*node.Node, error) {
	var out node.Node
	var lastSeen *time.Time
	err := row.Scan(
		&out.ID,
		&out.Name,
		&out.Region,
		&out.AgentBaseURL,
		&out.VPNEndpoint,
		&out.VPNEndpointHost,
		&out.VPNEndpointPort,
		&out.ServerPublicKey,
		&out.VPNSubnetCIDR,
		&out.RuntimeMetadata,
		&out.Status,
		&out.CurrentLoad,
		&out.UserCapacity,
		&lastSeen,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.LastSeenAt = lastSeen
	return &out, nil
}
