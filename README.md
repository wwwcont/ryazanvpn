# RyazanVPN Architecture (monorepo)

Этот репозиторий хранит общий код и инфраструктурные артефакты RyazanVPN.

## Схема сервисов

```text
[ Telegram / Admin / API clients ]
                |
                v
        +-------------------+
        |   Control-plane   |
        |  (HTTP API, DB)   |
        +-------------------+
          |           |
          |           +--> Postgres (state, billing, devices, traffic)
          +--> Redis (cache/state/replay guard)
          |
          v
    +-------------------+    (x N nodes)
    |    Node-agent     |----> runtime: AmneziaWG/Xray
    +-------------------+
```

## Текущая структура для физического split

- `split/control-plane/` — готовый standalone проект для отдельного репозитория control-plane.
- `split/node-agent/` — готовый standalone проект для отдельного репозитория node-agent.
- `shared/contracts/` — общий контракт node <-> control-plane.

## Как запускать после разделения

### Control-plane (в новом репозитории)

```bash
cp .env.example .env
make up
```

### Node-agent (в новом репозитории)

```bash
cp .env.example .env
make up
```

## Важно

- Корневые legacy compose/env/Makefile/Dockerfile удалены, чтобы не было двусмысленности.
- Канонические точки входа для разделения — только `split/*`.

## Готовые standalone директории для split

В репозитории добавлены две полностью копируемые директории:

- `split/control-plane/` — заготовка отдельного репозитория control-plane.
- `split/node-agent/` — заготовка отдельного репозитория node-agent.

Обе директории содержат исходники, `Dockerfile`, `docker-compose.yml`, `Makefile`, `.env.example`.
Их можно целиком перенести в новые репозитории и запускать независимо через `.env` в корне каждого нового репозитория.
