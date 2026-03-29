# Runbook: single-server deploy

## 1) Подготовка env

```bash
cp deploy/env/single-server.env.example .env.single.generated
```

Заполните секреты и проверьте ключевые значения:
- `CONTROL_PLANE_HTTP_ADDR=:8080`
- `NODE_AGENT_HTTP_ADDR=:8081`
- `PUBLIC_BASE_URL=https://api.rznvpn.online`
- `PUBLIC_DOMAIN=api.rznvpn.online`
- `RUNTIME_ADAPTER=amnezia_docker`
- `AMNEZIA_CONTAINER_NAME=amnezia-awg2`
- `AMNEZIA_INTERFACE_NAME=awg0`
- `DOCKER_BINARY_PATH=/usr/bin/docker`

## 2) Запуск стека

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
```

Ожидаемый порядок:
1. `postgres`
2. `redis`
3. `migrate`
4. `control-plane`
5. `node-agent`

## 3) Проверка состояния

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8081/health
curl http://localhost:8081/ready
```

Интерпретация:
- `node-agent /health` может быть `degraded`, если runtime недоступен.
- `node-agent /ready` возвращает `503`, если runtime недоступен.
- При этом сам `node-agent` процесс остаётся жив и не должен уходить в crash loop.

## 4) Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```
