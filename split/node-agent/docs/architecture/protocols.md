# Protocol orchestration contract

Supported protocols:
- `wireguard` (Speed / AmneziaWG runtime container)
- `xray` (Health / Reality runtime container)

## Node runtime ownership

Runtime containers are local infrastructure only.
Authoritative desired state is in control-plane DB.

`node-agent` applies and revokes runtime objects from desired state only.

## Desired state payload

`/nodes/desired-state` returns per access:
- access_id
- user_id
- device_id
- protocol
- peer_public_key
- preshared_key
- assigned_ip
- endpoint_params
- persistent_keepalive
- node_id
