# AmneziaWG Go runtime image

Container runtime for the `amnezia-awg` service.

## Config mounting
The runtime expects Amnezia/WireGuard config files from the mounted directory `/etc/amnezia`.

Config lookup order at startup:
1. `AMNEZIA_CONFIG_PATH` (if set)
2. `/etc/amnezia/${AMNEZIA_INTERFACE_NAME}.conf`
3. `/etc/amnezia/amneziawg.conf`
4. `/etc/amnezia/wg0.conf`
5. `/etc/amnezia/server.conf`

The container starts `amneziawg-go` for the interface (`awg0` by default), then applies the config via `awg setconf`.

## Compatibility mode
If native `awg` userspace tooling is unavailable, `/usr/local/bin/awg` falls back to `wg` while preserving runtime command compatibility for node-agent (`awg show`, `awg set`, `awg show all dump`).
