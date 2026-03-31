# AmneziaWG kernel-backed runtime image

Container runtime for the `amnezia-awg` service.

## Runtime model
This image does **not** run `amneziawg-go` userspace engine.
It is a thin control wrapper around a kernel-backed `amneziawg`/`wireguard` interface:

1. Create interface (`ip link add ... type amneziawg` with fallback to `wireguard`).
2. Apply config via `awg setconf` (wrapped to `wg` binary compatibility mode).
3. Keep container alive for `docker exec ... awg ...` operations from `node-agent`.

## Config mounting
The runtime expects Amnezia/WireGuard config files from mounted directory `/etc/amnezia`.

Config lookup order at startup:
1. `AMNEZIA_CONFIG_PATH` (if set)
2. `/etc/amnezia/${AMNEZIA_INTERFACE_NAME}.conf`
3. `/etc/amnezia/amneziawg.conf`
4. `/etc/amnezia/wg0.conf`
5. `/etc/amnezia/server.conf`

## Compatibility mode
`/usr/local/bin/awg` proxies to `wg` and strips Amnezia-only non-standard keys from `setconf`.
This preserves node-agent contract (`awg show`, `awg set`, `awg show all dump`) while using kernel-backed networking.
