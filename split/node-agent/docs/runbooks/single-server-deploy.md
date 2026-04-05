# Runbook: single-server (external Amnezia/Xray runtime)

## Модель

- `amnezia-awg` и `xray` уже запущены отдельно оператором.
- Репозиторий поднимает только:
  - `postgres`
  - `redis`
  - `migrate`
  - `control-plane`
  - `node-agent`

## 1) Подготовка

```bash
git clone <YOUR_REPO_URL> ryazanvpn
cd ryazanvpn
cp .env.example .env
```

Заполните `.env`:
- секреты (`POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `AGENT_HMAC_SECRET`, `ADMIN_API_SECRET`, `CONFIG_MASTER_KEY`, Telegram);
- runtime container names (`AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`, `XRAY_CONTAINER_NAME`);
- source path (`XRAY_SOURCE_CONFIG_PATH`) и ключи Reality из env (`XRAY_REALITY_PRIVATE_KEY`, `XRAY_REALITY_PUBLIC_KEY`);
- `VPN_PUBLIC_HOST`.

## 2) One-command запуск

```bash
make single
```

Команда сначала синхронизирует runtime-данные из `.env`, затем поднимает app stack.

## 3) Проверки

```bash
make ps-single
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/health
```

## 4) Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

Если webhook не доходит и в логах `control-plane` тишина:
- проверьте, что reverse-proxy (`caddy`/nginx) и `control-plane` подключены к одной Docker-сети `${RYAZANVPN_SHARED_NETWORK}`;
- для встроенного `caddy` в `docker-compose.yml` сервис `caddy` должен быть в сети `backend` вместе с `control-plane`.
- если `caddy` запущен на хосте (не в Docker), проксируйте на `127.0.0.1:8080` и проверьте, что опубликован порт `CONTROL_PLANE_PUBLISHED_PORT`.
