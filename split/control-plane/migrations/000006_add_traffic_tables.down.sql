DROP TRIGGER IF EXISTS trg_traffic_usage_daily_updated_at ON traffic_usage_daily;
DROP INDEX IF EXISTS idx_traffic_usage_daily_device_date;
DROP TABLE IF EXISTS traffic_usage_daily;
DROP INDEX IF EXISTS idx_device_traffic_snapshots_device_captured;
DROP TABLE IF EXISTS device_traffic_snapshots;
