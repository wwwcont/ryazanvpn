# Observability metrics: formulas and limitations

## Node throughput metrics

- `current_bps`: last sample from `node_throughput_samples`.
- `median_bps_1h`: percentile 50 over the last 1 hour.
- `p95_bps_1h`: percentile 95 over the last 1 hour.
- `peak_bps_24h`: max throughput over the last 24 hours.
- `link_utilization_percent`: `current_bps / NODE_LINK_CAPACITY_BPS * 100`.
- `heartbeat_fresh_seconds`: `now - vpn_nodes.last_seen_at`.

### Low utilization anomaly

Node is flagged as `low_utilization_anomaly=true` when:

1. `link_utilization_percent < 20`, and
2. `p95_bps_1h >= current_bps * 3`.

This helps surface unstable/underperforming nodes where current throughput is much lower than normal p95 baseline.

## Node quality tops

- `top-slow`: ordered by `p95_bps_1h` (descending), enriched with current utilization/anomaly.
- `top-errors`: ordered by 24h sum of:
  - failed apply reports (`node_apply_reports.status='failed'`),
  - failed/cancelled operations (`node_operations.status IN ('failed','cancelled')`).
- `top-noisy`: ordered by failed apply count and apply volume in last 24h, with `warn_error_rate_24h = failed_apply_24h / total_apply_24h`.

## User metrics

- `traffic_today_bytes`: sum for current date.
- `traffic_last_7_days_bytes`: rolling 7-day sum.
- `traffic_last_30_days_bytes`: rolling 30-day sum.
- `last_activity_at`: max `device_traffic_snapshots.captured_at` across user devices.
- `estimated_days_remaining`: `balance_kopecks / (DAILY_CHARGE_KOPECKS * billable_devices)`.

If balance, daily charge, or billable devices are zero, `estimated_days_remaining` is `null`.

## Limitations

- Throughput metrics depend on collector cadence and retention window; short outages can create sparse data.
- No synthetic ping/speed metrics are used; all numbers come from runtime counters and persisted samples.
- `top-noisy` currently reflects apply/report noise; application logs are not yet aggregated into this ranking.
