ALTER TABLE node_apply_reports
    DROP CONSTRAINT IF EXISTS node_apply_reports_operation_id_fkey;

ALTER TABLE node_apply_reports
    ALTER COLUMN operation_id TYPE TEXT USING operation_id::text;
