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
cp deploy/env/single-server.env.example .env.single.generated
```

Заполните `.env.single.generated`:
- секреты (`POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `AGENT_HMAC_SECRET`, `ADMIN_API_SECRET`, `CONFIG_MASTER_KEY`, Telegram);
- runtime container names (`AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`, `XRAY_CONTAINER_NAME`);
- source paths (`XRAY_SOURCE_CONFIG_PATH`, `XRAY_REALITY_PUBLIC_KEY_SOURCE_PATH`);
- `VPN_PUBLIC_HOST`.

## 2) One-command запуск

```bash
make single
```

Команда сначала синхронизирует runtime-данные из файлов в `.env`, затем поднимает app stack.

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
