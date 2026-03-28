# AmneziaWG runtime integration (safe scaffold)

## Цель
Этот каркас подготавливает `node-agent` к безопасной интеграции с реальным AmneziaWG/WireGuard runtime без выполнения сомнительных shell-команд.

## Runtime interfaces
В `internal/agent/runtime` выделены интерфейсы:
- `PeerManager`
  - `ApplyPeer(ctx, req)`
  - `RevokePeer(ctx, req)`
- `PeerStatsReader`
  - `ListPeerStats(ctx)`

`VPNRuntime` объединяет их + `Health(ctx)`.

## Ожидаемые бинарники/команды
Для shell adapter (`RUNTIME_ADAPTER=shell`) используются пути из env:
- `AWG_BINARY_PATH`
- `WG_BINARY_PATH`
- `IP_BINARY_PATH`

Для чтения peer stats (traffic counters) используется отдельная read-only команда:
- `RUNTIME_STATS_BINARY_PATH`
- `RUNTIME_STATS_ARGS` (CSV)

Пример:
- `RUNTIME_STATS_BINARY_PATH=/usr/bin/awg`
- `RUNTIME_STATS_ARGS=show,peer-stats,machine`

> Важно: adapter валидирует аргументы по allowlist regexp и не исполняет невалидные токены.

## Формат вывода для ListPeerStats
Shell scaffold ожидает строки формата:

`<device_access_id> <rx_total_bytes> <tx_total_bytes> [last_handshake_unix|-]`

Пример:

`da_abc123 1024 2048 1711600000`

## Маппинг peer -> device_access
Для корректного учёта трафика runtime обязан возвращать `device_access_id` напрямую в stats output.
Это ключевой идентификатор для записи snapshots/агрегации в control-plane.

## Включение через env
Node-agent:
- `RUNTIME_ADAPTER=shell`
- `RUNTIME_WORK_DIR=/var/lib/ryazanvpn/node-agent`
- `AWG_BINARY_PATH=/usr/bin/awg`
- `WG_BINARY_PATH=/usr/bin/wg`
- `IP_BINARY_PATH=/usr/sbin/ip`
- `RUNTIME_EXEC_TIMEOUT=10s`
- `RUNTIME_STATS_BINARY_PATH=/usr/bin/awg`
- `RUNTIME_STATS_ARGS=show,peer-stats,machine`

Если `RUNTIME_STATS_BINARY_PATH`/`RUNTIME_STATS_ARGS` не заданы, `ListPeerStats` возвращает `ErrNotImplemented` (без unsafe fallback).
