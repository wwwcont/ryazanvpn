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

make rebuild-single
```

Проверка:

```bash
make ps-single
curl http://localhost:8080/health
curl http://localhost:8081/health
```

Логи **не открываются автоматически** при запуске/пересборке.  
Открывайте их только вручную:

```bash
make logs-control   # только control-plane
make logs-agent     # только node-agent
make logs-single    # весь стек
```

Остановка:

```bash
make down-single
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
- путь задаётся через `DOCKER_BINARY_PATH` (рекомендуется `docker`, резолвится в PATH внутри container image)

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
- `XRAY_CONTAINER_NAME=ryazanvpn-xray`
- `DOCKER_BINARY_PATH=docker`
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

## Рекомендуемый operational flow (single-server)

```bash
make rebuild-single   # сборка + запуск в фоне (-d)
make ps-single        # статус контейнеров
make logs-control     # логи control-plane по запросу
make logs-agent       # логи node-agent по запросу
```

Опционально можно использовать скрипт-обёртку:

```bash
scripts/dev-single.sh up
scripts/dev-single.sh ps
scripts/dev-single.sh logs
scripts/dev-single.sh down
```

## Telegram UX выдачи конфига

После ввода валидного invite code бот выполняет полный pipeline:
1. Создаёт `access_grant`.
2. Создаёт `device` и `device_access`.
3. Применяет peer через `node-agent`.
4. Рендерит AmneziaWG/WireGuard `.conf`.
5. Отправляет конфиг в Telegram как document attachment (`rznvpn.conf`).

После успешной выдачи бот показывает inline-кнопки:
- `Скачать .conf` — повторная отправка файла (основной сценарий).
- `Показать QR` — отправка PNG QR для импорта.
- `Показать текст` — fallback с текстом конфига.

`/download/config/{token}` остаётся доступным для совместимости и внешнего download flow.

## Runtime model (external Amnezia/Xray)

Проект больше **не поднимает runtime VPN контейнеры** в single-server compose по умолчанию.

Новая модель:
- runtime (`amnezia-awg`, `xray`) уже существует отдельно и настроен оператором;
- `control-plane` + `node-agent` работают поверх этого runtime;
- `node-agent` обращается к runtime контейнерам через Docker CLI и имена контейнеров из `.env`.

Обязательные runtime env для такого режима:
- `AMNEZIA_CONTAINER_NAME`
- `AMNEZIA_INTERFACE_NAME`
- `XRAY_CONTAINER_NAME`
- `DOCKER_BINARY_PATH`

Рекомендуется также заполнить:
- `VPN_SERVER_PUBLIC_ENDPOINT`
- `VPN_SERVER_PUBLIC_KEY` (или `VPN_SERVER_PUBLIC_KEY_FILE`)
- `XRAY_PUBLIC_HOST`
- `XRAY_REALITY_PORT`
- `XRAY_REALITY_SERVER_NAME`
- `XRAY_REALITY_SHORT_ID`
- `XRAY_REALITY_PUBLIC_KEY` (или `XRAY_REALITY_PUBLIC_KEY_FILE`)

## Быстрый деплой: все сервисы на одной ноде (one-shot)

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты и runtime-параметры внешних контейнеров
docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
docker compose --env-file .env.single.generated -f docker-compose.single.yml ps
```

Проверка, что внешние runtime-контейнеры доступны по именам из `.env`:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
```

Подробно: `docs/runbooks/single-server-deploy.md`.

## Быстрый деплой: подключение дополнительной ноды к главной

На дополнительной ноде:

```bash
git clone <YOUR_REPO_URL> ryazanvpn
cd ryazanvpn
./scripts/bootstrap-node.sh
# заполните .env.node.generated (NODE_ID/NODE_TOKEN/CONTROL_PLANE_BASE_URL/AGENT_HMAC_SECRET)
docker compose --env-file .env.node.generated -f docker-compose.node.yml up -d --build
```

Подробно: `docs/runbooks/add-node.md`.

## Startup validation checklist

После `docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build` проверьте:

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml ps
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/health
docker logs $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q control-plane) --tail 100
docker logs $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q node-agent) --tail 100
docker exec $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q node-agent) docker version
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
```

Ожидания:
- `control-plane` в статусе `Up` и health endpoint отвечает `200`;
- `node-agent` в статусе `Up` и не ждёт compose-managed runtime;
- внешние runtime-контейнеры (`AMNEZIA_CONTAINER_NAME`, `XRAY_CONTAINER_NAME`) видны в `docker ps`;
- `node-agent` видит Docker API через `/var/run/docker.sock` и проходит runtime health.
