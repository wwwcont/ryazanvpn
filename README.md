# ryazanvpn MVP monorepo

Production-minded каркас VPN platform MVP на Go с двумя сервисами:

- `control-plane` (`cmd/control-plane`)
- `node-agent` (`cmd/node-agent`)

> В репозитории намеренно отсутствует бизнес-логика. Реализованы wiring, инфраструктурный каркас, конфигурация, миграции и health/readiness endpoints.

## Технологии

- Go `1.24+`
- HTTP router: `chi`
- PostgreSQL client: `pgx/v5` (`pgxpool`)
- Redis client: `go-redis/v9`
- Structured logging: `log/slog` в JSON
- Local orchestration: `docker compose`

## Структура

```text
.
├── cmd/
│   ├── control-plane/
│   └── node-agent/
├── internal/
│   ├── app/
│   ├── domain/
│   ├── infra/
│   └── transport/
├── migrations/
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

## Env-конфигурация

Оба сервиса настраиваются только через env:

- `HTTP_ADDR` (пример: `:8080` / `:8081`)
- `POSTGRES_URL` (обязательный)
- `REDIS_ADDR` (default: `redis:6379`)
- `REDIS_PASSWORD` (optional)
- `REDIS_DB` (default: `0`)
- `LOG_LEVEL` (`debug|info|warn|error`, default: `info`)
- `SHUTDOWN_TIMEOUT` (default: `15s`)
- `READINESS_TIMEOUT` (default: `2s`)

## Локальный запуск

1. Поднять всю среду:

```bash
make run
```

2. Проверить health endpoints:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8081/health
curl http://localhost:8081/ready
```


### Node-agent operations API

Node-agent предоставляет endpoints:

- `GET /health`
- `GET /ready`
- `POST /agent/v1/operations/apply-peer`
- `POST /agent/v1/operations/revoke-peer`

Операционные запросы защищены HMAC-подписью (headers `X-Agent-Timestamp`, `X-Agent-Signature`).

Env для node-agent:

- `AGENT_HMAC_SECRET` (обязательный)
- `AGENT_HMAC_MAX_SKEW` (default `5m`)

Payload для apply/revoke содержит:

- `operation_id`
- `device_access_id`
- `protocol`
- `peer_public_key`
- `assigned_ip`
- `keepalive`
- `endpoint_metadata` (optional)


### Control-plane -> node-agent integration (MVP)

В `internal/infra/nodeclient` добавлен HTTP client для вызова node-agent с:

- HMAC signing
- timeout
- retry logic

Env для control-plane:

- `NODE_AGENT_BASE_URL` (default `http://node-agent:8081`)
- `NODE_AGENT_HMAC_SECRET`
- `NODE_AGENT_TIMEOUT` (default `5s`)
- `NODE_AGENT_RETRIES` (default `2`)

Дополнительно для control-plane:

- `ADMIN_API_SECRET` — секрет для admin/debug endpoints (обязателен для использования `/admin/*`)
- `ADMIN_API_SECRET_HEADER` (default `X-Admin-Secret`)
- `NODE_HEALTH_POLL_INTERVAL` (default `15s`)
- `NODE_HEALTH_CHECK_TIMEOUT` (default `3s`)


### Config download for AmneziaWG

Control-plane поддерживает выдачу конфигов по токену:

- `GET /download/config/{token}`

Поведение:

- токен ищется по `sha256(token)`
- проверяются `expires_at` и `used_at`
- при успехе отдается `.conf` как attachment
- после успешной выдачи `used_at` заполняется

Для шифрования `device_accesses.config_blob_encrypted` используется AES-GCM сервис.

Env:

- `CONFIG_MASTER_KEY` — base64 AES key (16/24/32 bytes)

## Миграции

Используется CLI `golang-migrate` (`migrate` должен быть установлен локально).


Базовая схема MVP создаёт таблицы:

- `users`
- `invite_codes`
- `invite_code_activations`
- `vpn_nodes`
- `devices`
- `device_accesses`
- `node_operations`
- `config_download_tokens`
- `audit_logs`

Также применяется seed-миграция с invite codes `1111`, `2222`, `3333` и двумя активными VPN-нодами.

```bash
make migrate-up
make migrate-down
```

Переопределить DSN можно переменной:

```bash
POSTGRES_DSN='postgres://vpn:vpn@localhost:5432/vpn?sslmode=disable' make migrate-up
```

## Проверки

```bash
make test
make lint
```

## Domain + Repository layer (MVP)

Добавлены доменные модели и интерфейсы репозиториев:

- `internal/domain/user`
- `internal/domain/invitecode`
- `internal/domain/device`
- `internal/domain/access`
- `internal/domain/node`
- `internal/domain/audit`
- `internal/domain/operation`
- `internal/domain/token`

PostgreSQL-реализации на `pgx` находятся в `internal/infra/repository/postgres`.

Для invite-code usage flow добавлен транзакционный application service:

- `internal/app/invite_activation_service.go`

## Application use cases (MVP)

В `internal/app` добавлены use cases:

- `RegisterTelegramUser`
- `ActivateInviteCode`
- `CreateDeviceForUser`
- `AssignNodeForDevice`
- `CreateDeviceAccess`
- `RevokeDeviceAccess`
- `ListUserDevices`

А также выделены интерфейсы для:

- генерации ключей (`KeyGenerator`)
- аллокации IP (`IPAllocator`)
- выбора ноды (`NodeAssigner`)

## Graceful shutdown

Оба HTTP-сервера корректно обрабатывают `SIGINT/SIGTERM`:

- прекращают принимать новые запросы;
- завершают активные запросы в пределах `SHUTDOWN_TIMEOUT`;
- закрывают инфраструктурные подключения.

## Дальнейшее расширение

- Добавлять доменные модели и правила в `internal/domain`
- Сценарии use-case — в `internal/app`
- Интеграции/репозитории — в `internal/infra`
- HTTP/gRPC transport слой — в `internal/transport`


### Admin/debug endpoints

Control-plane предоставляет минимальный эксплуатационный набор для администрирования:

- `GET /admin/nodes`
- `GET /admin/users`
- `GET /admin/devices`
- `POST /admin/invite-codes`
- `POST /admin/invite-codes/{id}/revoke`

Все `/admin/*` endpoints защищены секретом из env (`ADMIN_API_SECRET`) через header `ADMIN_API_SECRET_HEADER` (по умолчанию `X-Admin-Secret`).

`POST /admin/invite-codes` создает новый invite code и пишет запись в `audit_logs`.
`POST /admin/invite-codes/{id}/revoke` отзывает invite code и также пишет `audit_logs`.

### Background worker: node health monitor

Внутри control-plane запускается background worker, который периодически опрашивает `GET {node.endpoint}/health` для каждой ноды:

- если endpoint недоступен или отвечает не-2xx, статус ноды переводится в `down`;
- если endpoint снова отвечает 2xx, статус возвращается в `active`.


### Node-agent runtime adapters (mock -> shell)

По умолчанию node-agent запускается с `RUNTIME_ADAPTER=mock`, что безопасно для MVP и тестирования.

Для подготовки к интеграции с реальным runtime:

1. Установите `RUNTIME_ADAPTER=shell`.
2. Задайте пути к бинарям и рабочей директории:
   - `RUNTIME_WORK_DIR`
   - `AWG_BINARY_PATH`
   - `WG_BINARY_PATH`
   - `IP_BINARY_PATH`
   - `RUNTIME_EXEC_TIMEOUT`
3. Убедитесь, что binaries существуют на диске (health-check shell runtime это проверяет).

Важно: в текущем MVP shell adapter содержит только шаблонную безопасную реализацию с TODO и structured logs; команды управления VPN peers (apply/revoke) намеренно не реализованы до финализации и валидации production runbook.
