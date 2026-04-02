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

## Self-contained node stack (production model)

Node host runs only repository-managed services:
- `node-agent`
- `amnezia-awg` runtime container (Speed)
- `xray` runtime container (Health)

Start with:

```bash
cp deploy/env/node.env.example .env.node.generated
./scripts/bootstrap-node.sh
```

Important:
- node-agent is the single runtime controller;
- do not use desktop Amnezia GUI on server hosts;
- no manual peer/client edits in runtime containers.

## Пошаговый single-host flow (VPN -> sync env -> control-plane -> node-agent)

Если хотите уйти от one-shot single режима, используйте 4 шага (один общий env в корне, например `.env.single.generated`):

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты только один раз

# 1) Поднять только runtime VPN контейнеры
make single-vpn-up

# 2) Считать runtime артефакты и синхронизировать ключи/порты в .env.single.generated
make single-runtime-sync

# 3) Поднять core сервис управления
make single-control-up

# 4) Поднять node-agent
make single-node-up
```

То же самое через скрипт:

```bash
scripts/dev-single.sh vpn-up
scripts/dev-single.sh sync
scripts/dev-single.sh core-up
scripts/dev-single.sh node-up
```

Для node-agent на той же машине используйте внутренний адрес control-plane:

```env
CONTROL_PLANE_BASE_URL=http://control-plane:8080
```

## Unified topology flow (single + distributed)

Теперь есть единый оркестрационный сценарий `scripts/topology-flow.sh`, где отличаются только:
- `TOPOLOGY_MODE` (`single-node`, `control-plane-only`, `node-only`, `distributed`);
- `INSTALL_ROLE` (для `distributed`: `control-plane`, `node`, `all`);
- адреса между `node-agent` и `control-plane` (`CONTROL_PLANE_BASE_URL`).

Фазы запуска одинаковые:
1. `runtime-up` — VPN runtime layer (`amnezia-awg`, `xray`);
2. `sync-env` — синхронизация env из runtime metadata/keys;
3. `control-up` — control-plane layer (`postgres`, `redis`, `migrate`, `control-plane`);
4. `node-up` — node layer (`node-agent`).

Пример single-node:

```bash
ENV_FILE=.env.single.generated TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh runtime-up
ENV_FILE=.env.single.generated TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh sync-env
ENV_FILE=.env.single.generated TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh control-up
ENV_FILE=.env.single.generated TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh node-up
```

Пример distributed (сервер A, control-plane-only):

```bash
ENV_FILE=.env.control.generated TOPOLOGY_MODE=control-plane-only ./scripts/topology-flow.sh control-up
```

Пример distributed (сервер B, node-only):

```bash
ENV_FILE=.env.node.generated TOPOLOGY_MODE=node-only ./scripts/topology-flow.sh runtime-up
ENV_FILE=.env.node.generated TOPOLOGY_MODE=node-only ./scripts/topology-flow.sh sync-env
ENV_FILE=.env.node.generated TOPOLOGY_MODE=node-only ./scripts/topology-flow.sh node-up
```

## Быстрый деплой: все сервисы на одной ноде (one-shot)

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты и XRAY_REALITY_PRIVATE_KEY
docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
docker compose --env-file .env.single.generated -f docker-compose.single.yml ps
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
docker logs ryazanvpn-xray --tail 50
docker exec amnezia-awg2 awg show awg0
docker exec $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q node-agent) docker version
```

Ожидания:
- `control-plane` в статусе `Up` и health endpoint отвечает `200`;
- `xray` в статусе `Up`, без ошибки `xray xray: unknown command`;
- `amnezia-awg` отдаёт `awg show` и интерфейс поднят kernel-backed runtime;
- `node-agent` видит Docker API через `/var/run/docker.sock` и проходит runtime health.
