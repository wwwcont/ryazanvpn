CREATE TABLE IF NOT EXISTS device_traffic_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    captured_at TIMESTAMPTZ NOT NULL,
    rx_total_bytes BIGINT NOT NULL,
    tx_total_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_device_traffic_snapshots_device_captured
    ON device_traffic_snapshots (device_id, captured_at DESC);

CREATE TABLE IF NOT EXISTS traffic_usage_daily (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    usage_date DATE NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_traffic_usage_daily_device_date UNIQUE (device_id, usage_date)
);

CREATE INDEX IF NOT EXISTS idx_traffic_usage_daily_device_date
    ON traffic_usage_daily (device_id, usage_date DESC);

CREATE TRIGGER trg_traffic_usage_daily_updated_at
BEFORE UPDATE ON traffic_usage_daily
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
