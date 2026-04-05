DROP TRIGGER IF EXISTS trg_traffic_usage_monthly_updated_at ON traffic_usage_monthly;
DROP TRIGGER IF EXISTS trg_traffic_usage_daily_protocol_updated_at ON traffic_usage_daily_protocol;

DROP TABLE IF EXISTS traffic_usage_monthly;
DROP TABLE IF EXISTS traffic_usage_daily_protocol;

DROP INDEX IF EXISTS idx_device_traffic_snapshots_access;

ALTER TABLE device_traffic_snapshots
    DROP COLUMN IF EXISTS last_handshake_at,
    DROP COLUMN IF EXISTS protocol,
    DROP COLUMN IF EXISTS vpn_node_id,
    DROP COLUMN IF EXISTS device_access_id;
