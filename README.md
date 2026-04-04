# RyazanVPN — production-minded MVP monorepo

Сервисы:
- `cmd/control-plane`
- `cmd/node-agent`

## Канонический production flow

1. Вы **сами** поднимаете и обслуживаете runtime контейнеры Amnezia/Xray.
2. В `.env.single.generated` указываете runtime-параметры контейнера Amnezia (`AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`) и путь к Xray config (`XRAY_SOURCE_CONFIG_PATH`).
3. Запускаете **одну команду**:

```bash
make single
```

Что делает `make single`:
- читает runtime-файлы Amnezia/Xray и обновляет env (`scripts/sync-runtime-from-configs.sh`);
- поднимает только app stack: `postgres`, `redis`, `migrate`, `control-plane`, `node-agent`.

## Быстрый старт

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты + container names + source paths
make single
```

Проверки:

```bash
make ps-single
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/health
```

## Откуда берутся runtime-данные

`scripts/sync-runtime-from-configs.sh` подтягивает:
- из runtime Amnezia через `docker exec ... awg show`: `VPN_SERVER_PUBLIC_KEY`, `AMNEZIA_PORT`;
- из runtime Amnezia через `ip addr show`: `VPN_SUBNET_CIDR` (если доступно);
- из `XRAY_SOURCE_CONFIG_PATH`: `XRAY_REALITY_PORT`, `XRAY_REALITY_SERVER_NAME`, `XRAY_REALITY_SHORT_ID`;
- из `XRAY_REALITY_PUBLIC_KEY_SOURCE_PATH`: `XRAY_REALITY_PUBLIC_KEY`;
- из `VPN_PUBLIC_HOST` + `AMNEZIA_PORT`: `VPN_SERVER_PUBLIC_ENDPOINT`.

Если путь к ключу не задан — можно оставить ручное значение в `.env`.

## Runtime logic (не меняли)

Сохранена рабочая логика:
- add/remove peer в Amnezia через `node-agent`;
- модификация Xray config и добавление клиента;
- выдача/скачивание клиентских конфигов через `control-plane`;
- Telegram pipeline выдачи конфига.

## Build/Test

```bash
go test ./...
```
