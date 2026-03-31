# Runbook: развертывание на одной ноде (все сервисы на одном сервере)

## Что поднимается

`docker-compose.single.yml` запускает:
1. `postgres`
2. `redis`
3. `migrate`
4. `control-plane`
5. `amnezia-awg`
6. `xray`
7. `node-agent`

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
- `XRAY_REALITY_PRIVATE_KEY`

## Этап 4. Запуск стека

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml up -d --build
```

## Этап 5. Проверки

```bash
docker compose --env-file .env.single.generated -f docker-compose.single.yml ps
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8080/ready
curl -fsS http://localhost:8081/health
curl -fsS http://localhost:8081/ready
```

Проверка Xray:

```bash
docker logs ryazanvpn-xray --tail 50
```

Ожидается отсутствие ошибки `xray xray: unknown command`.

Проверка AmneziaWG runtime (kernel-backed):

```bash
docker exec amnezia-awg2 awg show awg0
docker exec amnezia-awg2 ip link show awg0
```

Проверка, что node-agent может работать с runtime через Docker CLI:

```bash
docker exec $(docker compose --env-file .env.single.generated -f docker-compose.single.yml ps -q node-agent) docker version
```

## Этап 6. Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

## Важно

- Не управляйте runtime вручную через desktop Amnezia.
- `node-agent` — единственный runtime controller для `amnezia-awg` и `xray`.
