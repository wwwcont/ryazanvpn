# RyazanVPN — production-minded MVP monorepo

Монорепозиторий с двумя сервисами:
- `cmd/control-plane`
- `cmd/node-agent`

Архитектурные слои:
- `internal/domain` — доменные модели и интерфейсы.
- `internal/app` — use cases и workers.
- `internal/infra` — postgres/redis/telegram/runtime adapters.
- `internal/transport` — HTTP transport (control-plane и node-agent).

## Режимы запуска

### 1) Single-server
Используйте `docker-compose.single.yml`:
- `control-plane`
- `postgres`
- `redis`
- `migrate`
- `node-agent`
- `caddy`

```bash
make run-single
```

### 2) Backend + multi-node
Backend (`docker-compose.backend.yml`):
- `control-plane`, `postgres`, `redis`, `migrate`, `caddy`

```bash
make run-backend
```

Node (`docker-compose.node.yml`):
- `node-agent`

```bash
make run-node
```

## Env файлы

Шаблоны:
- `deploy/env/single-server.env.example`
- `deploy/env/backend.env.example`
- `deploy/env/node.env.example`

Сгенерированные:
- `.env.single.generated`
- `.env.backend.generated`
- `.env.node.generated`

> ⚠️ `.env.single.generated` содержит чувствительные данные (секреты, ключи, токены) и **не должен попадать в публичный git**.

## Telegram flow

Поддерживается webhook flow:
- `/start` и русскоязычное приветствие
- user menu: Ввести код / Мой доступ / Получить конфиг / Удалить устройство / Помощь
- admin menu (по `TELEGRAM_ADMIN_IDS`):
  - Создать 1 код
  - Создать пачку кодов
  - Последние коды
  - Активные пользователи
  - Статус нод
  - Статистика пользователя
  - Отозвать доступ

Invite code: 4 цифры, одноразовый, выдается админом, при активации создает grant на 30 дней.

## Amnezia runtime

Режим `RUNTIME_ADAPTER=amnezia_docker` использует:
- `AMNEZIA_CONTAINER_NAME`
- `AMNEZIA_INTERFACE_NAME`
- `DOCKER_BINARY_PATH`

Операции:
- Health
- ApplyPeer
- RevokePeer
- ListPeerStats (`awg show all dump` parser)

## Metrics API (под будущий сайт)

- `GET /api/v1/metrics/overview`
- `GET /api/v1/metrics/nodes`
- `GET /api/v1/metrics/users/{id}`

Latency/speed сейчас возвращаются как `null/not_available` (честный placeholder).

## Миграции и проверки

```bash
make migrate-up
make migrate-down
make test
make lint
go build ./cmd/control-plane
go build ./cmd/node-agent
```

## Runbooks

- `docs/runbooks/single-server-deploy.md`
- `docs/runbooks/backend-deploy.md`
- `docs/runbooks/node-deploy.md`
- `docs/runbooks/rotate-telegram-token.md`
- `docs/runbooks/rotate-node-agent-hmac.md`
- `docs/runbooks/add-new-node.md`
