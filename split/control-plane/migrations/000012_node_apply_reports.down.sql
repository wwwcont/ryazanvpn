DROP INDEX IF EXISTS idx_node_apply_reports_operation_id;
DROP INDEX IF EXISTS idx_node_apply_reports_node_created_at;
DROP TABLE IF EXISTS node_apply_reports;

ALTER TABLE node_operations DROP CONSTRAINT IF EXISTS node_operations_status_check;
ALTER TABLE node_operations
    ADD CONSTRAINT node_operations_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled'));
