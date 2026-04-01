ALTER TABLE node_operations
    ALTER COLUMN id DROP DEFAULT;

ALTER TABLE node_operations
    ALTER COLUMN id TYPE TEXT USING id::text;

ALTER TABLE node_operations
    ALTER COLUMN id SET DEFAULT gen_random_uuid()::text;
