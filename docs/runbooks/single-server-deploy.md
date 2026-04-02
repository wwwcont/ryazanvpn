# Runbook: развертывание на одной ноде (все сервисы на одном сервере)

## Что поднимается

`docker-compose.single.yml` запускает:
1. `postgres`
2. `redis`
3. `migrate`
4. `control-plane`
5. `node-agent`

`amnezia-awg` и `xray` должны быть подняты отдельно (внешний runtime).

## Этап 1. Подготовка сервера

```bash
sudo apt update
sudo apt install -y git docker.io docker-compose-plugin
sudo systemctl enable --now docker
```

## Этап 2. Клонирование репы

```bash
git clone <YOUR_REPO_URL> ryazanvpn
cd ryazanvpn
```

## Этап 3. Подготовка env

```bash
cp deploy/env/single-server.env.example .env.single.generated
```

Обязательно заполните:
- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`
- `AGENT_HMAC_SECRET`
- `NODE_AGENT_HMAC_SECRET`
- `NODE_REGISTRATION_TOKEN`
- `ADMIN_API_SECRET`
- `CONFIG_MASTER_KEY`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `AMNEZIA_CONTAINER_NAME` (имя уже существующего контейнера AmneziaWG)
- `AMNEZIA_INTERFACE_NAME`
- `XRAY_CONTAINER_NAME` (имя уже существующего контейнера Xray)
- runtime-параметры (`VPN_SERVER_PUBLIC_ENDPOINT`, `VPN_SERVER_PUBLIC_KEY` или `VPN_SERVER_PUBLIC_KEY_FILE`, `XRAY_PUBLIC_HOST`, `XRAY_REALITY_PORT`, `XRAY_REALITY_SERVER_NAME`, `XRAY_REALITY_SHORT_ID`, `XRAY_REALITY_PUBLIC_KEY` или `XRAY_REALITY_PUBLIC_KEY_FILE`)

## Этап 4. Проверка внешнего runtime

Перед запуском сервисного стека убедитесь, что внешние runtime-контейнеры действительно работают:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
```

## Этап 5. Запуск стека

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
```

## Этап 6. Проверки

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml ps
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8080/ready
curl -fsS http://localhost:8081/health
curl -fsS http://localhost:8081/ready
```

Проверка, что node-agent может работать с runtime через Docker CLI:

```bash
docker exec $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q node-agent) docker version
```

## Этап 7. Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

## Важно

- В этом сценарии runtime (`amnezia-awg`, `xray`) — внешний источник истины, а не часть compose этого репозитория.
- `node-agent` управляет peer/client-операциями через Docker CLI в уже существующих контейнерах по именам из `.env`.
