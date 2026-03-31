ALTER TABLE node_operations DROP CONSTRAINT IF EXISTS node_operations_status_check;
ALTER TABLE node_operations
    ADD CONSTRAINT node_operations_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'pending', 'applied', 'retrying', 'manual_intervention_required'));

CREATE TABLE IF NOT EXISTS node_apply_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES vpn_nodes (id) ON DELETE CASCADE,
    operation_id UUID REFERENCES node_operations (id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
    error_message TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_apply_reports_node_created_at
    ON node_apply_reports (node_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_node_apply_reports_operation_id
    ON node_apply_reports (operation_id);
