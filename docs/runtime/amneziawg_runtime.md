# AmneziaWG runtime integration

## Цель
`node-agent` использует runtime adapter `RUNTIME_ADAPTER=amnezia_docker` для управления AmneziaWG через Docker-контейнер, но сам dataplane должен быть kernel-backed.

## Runtime interfaces
В `internal/agent/runtime` выделены интерфейсы:
- `PeerManager`
  - `ApplyPeer(ctx, req)`
  - `RevokePeer(ctx, req)`
- `PeerStatsReader`
  - `ListPeerStats(ctx)`

`VPNRuntime` объединяет их + `Health(ctx)`.

## Kernel-backed модель
Контейнер `amnezia-awg` — это thin wrapper:
- поднимает интерфейс `amneziawg` (fallback `wireguard`) через `ip link add`;
- применяет стартовый конфиг через `awg setconf`;
- дальше принимает runtime-команды от `node-agent` (`docker exec ... awg ...`).

`amneziawg-go` userspace engine не используется как основной runtime путь.

## Ожидаемые env для docker adapter
- `DOCKER_BINARY_PATH` (рекомендуется `docker`, резолвится через `$PATH`)
- `AMNEZIA_CONTAINER_NAME` (пример: `amnezia-awg2`)
- `AMNEZIA_INTERFACE_NAME` (пример: `awg0`)
- `XRAY_CONTAINER_NAME` (пример: `ryazanvpn-xray`)

Если `node-agent` запущен в Docker, нужен только сокет:
- `- /var/run/docker.sock:/var/run/docker.sock`

Монтировать бинарник `/usr/bin/docker` с хоста не требуется.

## Формат вывода для ListPeerStats
Используется `docker exec <container> awg show all dump`.
Парсятся peer-строки формата wireguard dump:
- public key
- preshared key
- endpoint
- allowed ips
- latest handshake unix timestamp
- rx bytes
- tx bytes
- persistent keepalive

## Включение через env
Node-agent:
- `RUNTIME_ADAPTER=amnezia_docker`
- `RUNTIME_WORK_DIR=/var/lib/ryazanvpn/node-agent`
- `DOCKER_BINARY_PATH=docker`
- `AMNEZIA_CONTAINER_NAME=amnezia-awg2`
- `AMNEZIA_INTERFACE_NAME=awg0`
- `XRAY_CONTAINER_NAME=ryazanvpn-xray`
- `RUNTIME_EXEC_TIMEOUT=10s`
