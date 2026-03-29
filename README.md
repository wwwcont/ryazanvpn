# RyazanVPN — production-minded MVP monorepo

Монорепозиторий с двумя сервисами:
- `cmd/control-plane`
- `cmd/node-agent`

Архитектурные слои:
- `internal/domain` — модели и доменные интерфейсы.
- `internal/app` — use cases/workers.
- `internal/infra` — postgres/redis/telegram/runtime adapters.
- `internal/transport` — HTTP API.

---

## Что уже подставлено из ваших данных

Подставлены в `.env.single.generated` и `.env.backend.generated`:
- `TELEGRAM_BOT_TOKEN=8799406539:AAG3s6Gylc4zWuS30SYy4gbeZabw0mMbOpI`
- `TELEGRAM_ADMIN_IDS=7423296715`
- `TELEGRAM_WEBHOOK_SECRET=webhooksecret123`
- `PUBLIC_BASE_URL=https://api.rznvpn.online`
- `VPN_SERVER_PUBLIC_ENDPOINT=193.29.224.182:41475`
- `VPN_SERVER_PUBLIC_KEY=iyuNicNyxL3EWzP3JgRJdKozE8TXOArEU6TGcMoK5CU=`
- `VPN_SUBNET_CIDR=10.8.1.0/24`
- `RUNTIME_ADAPTER=amnezia_docker`
- `AMNEZIA_CONTAINER_NAME=amnezia-awg2`
- `AMNEZIA_INTERFACE_NAME=awg0`
- `DOCKER_BINARY_PATH=/usr/bin/docker`

---

## Что вам нужно заполнить вручную

В `.env.single.generated`, `.env.backend.generated`, `.env.node.generated` оставлены `CHANGE_ME_*`.

Обязательные для выбора вручную:
1. `POSTGRES_PASSWORD`
2. `REDIS_PASSWORD`
3. `NODE_AGENT_HMAC_SECRET` (`AGENT_HMAC_SECRET` на нодах должен быть **точно таким же**)
4. `ADMIN_API_SECRET`
5. `CONFIG_MASTER_KEY`

### Как выбрать значения
- `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `NODE_AGENT_HMAC_SECRET`, `ADMIN_API_SECRET`:
  - длина 32+ символов, случайные, без пробелов.
  - пример генерации: `openssl rand -base64 48`
- `CONFIG_MASTER_KEY`:
  - base64 от 32 случайных байт (AES-256).
  - пример: `openssl rand -base64 32`

После генерации просто замените `CHANGE_ME_*` в env-файлах.

> ⚠️ `.env.single.generated` содержит реальные секреты. Не коммитьте его в публичный репозиторий.

---

## Запуск: single-server (всё на одном сервере)

Компоненты:
- `control-plane`
- `postgres`
- `redis`
- `migrate`
- `node-agent`
- `caddy`

Команда:
```bash
make run-single
```

Проверка:
```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8081/health
curl http://localhost:8081/ready
```

Telegram webhook:
```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

---

## Запуск: multi-node (backend + много VPN нод)

### 1) Backend сервер
Компоненты:
- `control-plane`
- `postgres`
- `redis`
- `migrate`
- `caddy`

Команда:
```bash
make run-backend
```

### 2) Каждая VPN-нода отдельно
Компоненты:
- `node-agent`

Команда:
```bash
make run-node
```

### 3) Регистрация ноды в БД
Для каждой новой ноды заполните `vpn_nodes`:
- `agent_base_url` — адрес node-agent для control-plane
- `vpn_endpoint_host` / `vpn_endpoint_port` — публичный endpoint для клиентов
- `server_public_key`
- `vpn_subnet_cidr`
- `status=active`

---

## Масштабирование

### Горизонтально по нодам
1. Поднимаете новый `node-agent`.
2. Даёте ему общий `AGENT_HMAC_SECRET`.
3. Добавляете запись в `vpn_nodes`.
4. Проверяете `/api/v1/metrics/nodes` и Telegram «Статус нод».

### Вертикально backend
- Увеличиваете ресурсы backend сервера (CPU/RAM/IOPS).
- Выносите Postgres на managed/replicated инстанс при росте.
- Redis — отдельный инстанс/managed service.

---

## Основные endpoints

- Health:
  - `GET /health`
  - `GET /ready`
- Telegram webhook:
  - `POST /internal/telegram/webhook`
- Config download:
  - `GET /download/config/{token}`
- Metrics API:
  - `GET /api/v1/metrics/overview`
  - `GET /api/v1/metrics/nodes`
  - `GET /api/v1/metrics/users/{id}`

---

## Make targets

```bash
make run-single
make run-backend
make run-node
make migrate-up
make migrate-down
make test
make lint
```

---

## Build/Test

```bash
go build ./cmd/control-plane
go build ./cmd/node-agent
go test ./...
```
