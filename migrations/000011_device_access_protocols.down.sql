DROP INDEX IF EXISTS idx_device_accesses_protocol_status;

ALTER TABLE device_accesses DROP CONSTRAINT IF EXISTS uq_device_accesses_device_node_protocol;
ALTER TABLE device_accesses
    ADD CONSTRAINT uq_device_accesses_device_node UNIQUE (device_id, vpn_node_id);

ALTER TABLE device_accesses
    DROP COLUMN IF EXISTS protocol;
