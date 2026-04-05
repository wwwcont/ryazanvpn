ALTER TABLE node_operations
    ALTER COLUMN id DROP DEFAULT;

ALTER TABLE node_operations
    ALTER COLUMN id TYPE UUID USING (
        CASE
            WHEN id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$' THEN id::uuid
            ELSE gen_random_uuid()
        END
    );

ALTER TABLE node_operations
    ALTER COLUMN id SET DEFAULT gen_random_uuid();
