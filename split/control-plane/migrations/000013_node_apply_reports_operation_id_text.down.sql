DELETE FROM node_apply_reports WHERE operation_id IS NOT NULL AND operation_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$';

ALTER TABLE node_apply_reports
    ALTER COLUMN operation_id TYPE UUID USING NULLIF(operation_id, '')::uuid;

ALTER TABLE node_apply_reports
    ADD CONSTRAINT node_apply_reports_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES node_operations (id) ON DELETE SET NULL;
