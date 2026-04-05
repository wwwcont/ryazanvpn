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

## Текущая структура для разделения по сервисам

- `services/control-plane/` — всё для запуска и эксплуатации control-plane (свой `README`, `docker-compose.yml`, `.env.example`, `Makefile`).
- `services/node-agent/` — всё для запуска и эксплуатации node-agent (свой `README`, `docker-compose.yml`, `.env.example`, `Makefile`).
- `shared/contracts/` — общий контракт node <-> control-plane.

## Где что запускать

### Control-plane

```bash
cp services/control-plane/.env.example services/control-plane/.env.generated
make -C services/control-plane up
```

### Node-agent

```bash
cp services/node-agent/.env.example services/node-agent/.env.generated
make -C services/node-agent up
```

## Важно

- Это всё ещё единый monorepo, но operational артефакты разложены по сервисным директориям для дальнейшего физического split в отдельные git-репозитории.
- Исторические compose/env в корне можно считать legacy-совместимостью; канонические entrypoints теперь в `services/*`.
