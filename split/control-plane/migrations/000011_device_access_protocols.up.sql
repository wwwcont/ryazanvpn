ALTER TABLE device_accesses
    ADD COLUMN IF NOT EXISTS protocol TEXT NOT NULL DEFAULT 'wireguard';

ALTER TABLE device_accesses DROP CONSTRAINT IF EXISTS uq_device_accesses_device_node;
ALTER TABLE device_accesses
    ADD CONSTRAINT uq_device_accesses_device_node_protocol UNIQUE (device_id, vpn_node_id, protocol);

CREATE INDEX IF NOT EXISTS idx_device_accesses_protocol_status
    ON device_accesses (protocol, status);
