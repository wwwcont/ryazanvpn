# Logging format, error taxonomy, and export limits

## Structured record (NDJSON)

Each log line is a JSON object:

- `ts` (RFC3339 timestamp)
- `service` (`control-plane`, `node-agent`, etc.)
- `severity` (`info`, `warn`, `error`)
- `error_code` (taxonomy code, optional)
- `request_id` (if available)
- `node_id` (if available)
- `user_id` (if available)
- `device_id` (if available)
- `operation_id` (if available)
- `message` (short human-readable summary)

## Taxonomy examples

- Reconcile/apply:
  - `E_RECONCILE_PEER`
  - `E_RECONCILE_TRAFFIC`
  - `E_HTTP_4XX`, `E_HTTP_5XX`
- Billing:
  - `E_BILLING_SET`
  - `E_BILLING_ADJUST`
  - `E_BILLING_DAILY`
- Telegram:
  - `E_TELEGRAM_FLOW`

## Export API

`GET /admin/logs/export`

Query params:

- `from` / `to` (RFC3339)
- `service`
- `node_id`
- `level`
- `limit` (max 10000, default 5000)

Response:

- `application/x-ndjson` attachment
- header `X-Export-Count`

## Retention/compression

- Active files: `ops-YYYYMMDD.ndjson`
- Older than 24h are compressed to `.ndjson.gz`
- Compressed logs older than `OPS_LOG_RETENTION` are removed.

## Limitations

- Export currently reads from local filesystem store of the control-plane instance.
- Multi-node aggregation requires external log shipping/indexing.
