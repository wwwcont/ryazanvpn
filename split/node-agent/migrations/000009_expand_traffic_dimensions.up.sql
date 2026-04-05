ALTER TABLE device_traffic_snapshots
    ADD COLUMN IF NOT EXISTS device_access_id UUID REFERENCES device_accesses (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS vpn_node_id UUID REFERENCES vpn_nodes (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS protocol TEXT NOT NULL DEFAULT 'wireguard',
    ADD COLUMN IF NOT EXISTS last_handshake_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_device_traffic_snapshots_access
    ON device_traffic_snapshots (device_access_id, captured_at DESC);

CREATE TABLE IF NOT EXISTS traffic_usage_daily_protocol (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    vpn_node_id UUID REFERENCES vpn_nodes (id) ON DELETE SET NULL,
    protocol TEXT NOT NULL,
    usage_date DATE NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_traffic_usage_daily_protocol UNIQUE (device_id, vpn_node_id, protocol, usage_date)
);

CREATE INDEX IF NOT EXISTS idx_traffic_usage_daily_protocol_lookup
    ON traffic_usage_daily_protocol (device_id, protocol, usage_date DESC);

CREATE TABLE IF NOT EXISTS traffic_usage_monthly (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    vpn_node_id UUID REFERENCES vpn_nodes (id) ON DELETE SET NULL,
    protocol TEXT NOT NULL,
    usage_month DATE NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_traffic_usage_monthly UNIQUE (device_id, vpn_node_id, protocol, usage_month)
);

CREATE INDEX IF NOT EXISTS idx_traffic_usage_monthly_lookup
    ON traffic_usage_monthly (device_id, protocol, usage_month DESC);

CREATE TRIGGER trg_traffic_usage_daily_protocol_updated_at
BEFORE UPDATE ON traffic_usage_daily_protocol
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_traffic_usage_monthly_updated_at
BEFORE UPDATE ON traffic_usage_monthly
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
