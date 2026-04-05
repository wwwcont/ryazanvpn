# Node-agent service

`services/node-agent` — папка запуска и эксплуатации node-agent и runtime-контейнеров ноды.

## Что находится в папке

- `Makefile` — build/test/up/down для node-agent.
- `.env.example` — пример env для node-runtime + node-agent.
- `docker-compose.yml` — compose-стек node (`amnezia-awg`, `xray`, `node-agent`).

## Быстрый запуск

```bash
cp services/node-agent/.env.example services/node-agent/.env.generated
make -C services/node-agent up
```

Остановка:

```bash
make -C services/node-agent down
```

## Проверки

```bash
curl -fsS http://localhost:8081/health
```

## Что настраивать обязательно

- `CONTROL_PLANE_BASE_URL`
- `AGENT_HMAC_SECRET` / `NODE_AGENT_HMAC_SECRET`
- `NODE_ID`, `NODE_TOKEN`, `NODE_REGISTRATION_TOKEN`
- runtime-переменные:
  - `AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`, `AMNEZIA_PORT`, `AMNEZIA_SUBNET`
  - `XRAY_CONTAINER_NAME`, `XRAY_SOURCE_CONFIG_PATH`, `XRAY_REALITY_*`
