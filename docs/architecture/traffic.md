# Traffic collection architecture

## Goals

- Correct collection for:
  - AmneziaWG (`awg show all dump`)
  - Xray (stats source via runtime stats adapter/API)
- Deterministic linkage:
  - `traffic -> user -> device -> protocol -> node`

## Data model

Three storage layers:

1. **Raw snapshots**: `device_traffic_snapshots`
   - per capture point counters
   - includes `device_access_id`, `vpn_node_id`, `protocol`, `last_handshake_at`
2. **Daily aggregates**:
   - legacy: `traffic_usage_daily`
   - dimensional: `traffic_usage_daily_protocol` (device + node + protocol + date)
3. **Monthly aggregates**:
   - `traffic_usage_monthly` (device + node + protocol + month)

## Collection flow

1. control-plane worker polls node-agent `/agent/v1/traffic/counters`.
2. Counter is resolved to access/device by:
   - `device_access_id`, or
   - fallback `allowed_ip + node_id`.
3. Snapshot is stored (including handshake and protocol).
4. Delta is computed against previous snapshot.
5. Delta is added to daily and monthly aggregates.

## API

- `GET /users/{id}/traffic`
  - total bytes
  - breakdown by protocol
  - monthly by protocol

## Signature/auth

Node traffic polling uses shared HMAC secret and common signing algorithm
(`X-Agent-Timestamp` + `X-Agent-Signature`) between control-plane and node-agent.
