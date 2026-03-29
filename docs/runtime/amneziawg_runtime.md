# AmneziaWG runtime integration

## Цель
`node-agent` поддерживает runtime adapter `RUNTIME_ADAPTER=amnezia_docker` для управления AmneziaWG внутри Docker-контейнера.

## Runtime interfaces
В `internal/agent/runtime` выделены интерфейсы:
- `PeerManager`
  - `ApplyPeer(ctx, req)`
  - `RevokePeer(ctx, req)`
- `PeerStatsReader`
  - `ListPeerStats(ctx)`

`VPNRuntime` объединяет их + `Health(ctx)`.

## Ожидаемые бинарники/команды
Для Docker adapter используются env:
- `DOCKER_BINARY_PATH` (default `/usr/bin/docker`)
- `AMNEZIA_CONTAINER_NAME` (пример: `amnezia-awg2`)
- `AMNEZIA_INTERFACE_NAME` (пример: `awg0`)

Если `node-agent` запущен в Docker, нужно пробросить в контейнер:
- `- /var/run/docker.sock:/var/run/docker.sock`

Docker daemon внутри контейнера не требуется: `node-agent` использует Docker CLI в контейнере и Docker socket хоста.

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

## Маппинг peer -> device_access
Трафик в control-plane сначала маппится по `device_access_id` (если runtime уже знает связь `peer_public_key -> device_access_id`), а затем fallback по `allowed_ip` + `vpn_node_id`.

## Включение через env
Node-agent:
- `RUNTIME_ADAPTER=amnezia_docker`
- `RUNTIME_WORK_DIR=/var/lib/ryazanvpn/node-agent`
- `DOCKER_BINARY_PATH=/usr/bin/docker`
- `AMNEZIA_CONTAINER_NAME=amnezia-awg2`
- `AMNEZIA_INTERFACE_NAME=awg0`
- `RUNTIME_EXEC_TIMEOUT=10s`
