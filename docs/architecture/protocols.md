# Protocol orchestration contract

Supported protocols:
- `wireguard` (Speed / AmneziaWG)
- `xray` (Health / Reality)

## Desired state payload

`/nodes/desired-state` returns per `device_access` item with:
- `access_id`
- `user_id`
- `device_id`
- `protocol`
- `peer_public_key`
- `preshared_key` (nullable)
- `assigned_ip`
- `endpoint_params`
- `persistent_keepalive`
- `node_id`

## Apply/revoke invariants

- Control-plane always passes protocol from DB.
- Revoke payload includes real `peer_public_key` and `protocol`.
- Node-agent must not assume default protocol.
