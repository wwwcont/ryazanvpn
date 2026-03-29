# RyazanVPN — production-minded MVP monorepo

Монорепозиторий с двумя сервисами:
- `cmd/control-plane`
- `cmd/node-agent`

## Single-server режим (рекомендованный для 1 ноды)

В single-server стеке запускаются:
- `postgres`
- `redis`
- `migrate`
- `control-plane` (HTTP `:8080`)
- `node-agent` (HTTP `:8081`)

Запуск:

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты

docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
```

Проверка:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8081/health
curl http://localhost:8081/ready
```

## Разделение env для control-plane и node-agent

Используются service-scoped переменные:
- `CONTROL_PLANE_HTTP_ADDR` — адрес `control-plane` (по умолчанию `:8080`)
- `NODE_AGENT_HTTP_ADDR` — адрес `node-agent` (по умолчанию `:8081`)

Для обратной совместимости остаётся `HTTP_ADDR`, но scoped-переменные имеют приоритет.

В `docker-compose.single.yml` оба сервиса читают общий `env_file`, но критичные runtime/listen env задаются явно per-service.

## Runtime для Amnezia Docker

Для `RUNTIME_ADAPTER=amnezia_docker`:
- `node-agent` использует Docker socket хоста: `/var/run/docker.sock:/var/run/docker.sock`
- внутри image `node-agent` должен быть Docker CLI (`docker-cli`)
- путь задаётся через `DOCKER_BINARY_PATH` (рекомендуется `/usr/bin/docker`)

Если runtime недоступен при старте, `node-agent` больше не падает в crash loop:
- HTTP сервер продолжает работать
- `/health` показывает degraded
- `/ready` возвращает `503` до восстановления runtime
- operation endpoints возвращают `503 runtime unavailable`

## Обязательные env для single-server

Минимум:
- `CONTROL_PLANE_HTTP_ADDR=:8080`
- `NODE_AGENT_HTTP_ADDR=:8081`
- `NODE_AGENT_BASE_URL=http://node-agent:8081`
- `RUNTIME_ADAPTER=amnezia_docker`
- `AMNEZIA_CONTAINER_NAME=amnezia-awg2`
- `AMNEZIA_INTERFACE_NAME=awg0`
- `DOCKER_BINARY_PATH=/usr/bin/docker`
- `VPN_SERVER_PUBLIC_ENDPOINT=193.29.224.182:41475`
- `VPN_SERVER_PUBLIC_KEY=iyuNicNyxL3EWzP3JgRJdKozE8TXOArEU6TGcMoK5CU=`
- `VPN_SUBNET_CIDR=10.8.1.0/24`
- `VPN_CLIENT_ALLOWED_IPS=0.0.0.0/0,::/0`

И секреты (заполняются вручную):
- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`
- `AGENT_HMAC_SECRET`
- `NODE_AGENT_HMAC_SECRET`
- `ADMIN_API_SECRET`
- `CONFIG_MASTER_KEY`

## Build/Test

```bash
go build ./cmd/control-plane
go build ./cmd/node-agent
go test ./...
```
