# Single-server deployment guide (all services on one server)

Этот сценарий — для одного купленного сервера, где запускаются **все компоненты**:
- PostgreSQL
- Redis
- control-plane
- node-agent

## 1) Подготовка сервера

```bash
sudo apt update
sudo apt install -y git curl ca-certificates ufw
```

(Опционально) установить Docker + compose plugin:

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# re-login
```

## 2) Клонирование проекта

```bash
git clone <YOUR_REPO_URL> /opt/ryazanvpn
cd /opt/ryazanvpn
```

## 3) Подготовка env

1. Скопируйте шаблон:

```bash
cp deploy/env/all-in-one.env.example /etc/ryazanvpn/all-in-one.env
sudo chmod 600 /etc/ryazanvpn/all-in-one.env
```

2. Заполните **все** необходимые значения.

Минимально критичные поля:
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_URL`
- `REDIS_ADDR`, `REDIS_DB`
- `ADMIN_API_SECRET`
- `CONFIG_MASTER_KEY`
- `NODE_AGENT_HMAC_SECRET` + `AGENT_HMAC_SECRET` (должны совпадать)
- `PUBLIC_BASE_URL`
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_ADMIN_IDS`

## 4) Вариант A: запуск через docker compose (рекомендуется для быстрого старта)

### 4.1 Подготовьте `.env` рядом с `docker-compose.yml`

Создайте `/opt/ryazanvpn/.env` на основе `/etc/ryazanvpn/all-in-one.env`.

### 4.2 Запуск

```bash
docker compose up -d --build
```

### 4.3 Миграции

Запустите миграции из control-plane контейнера или локально:

```bash
make migrate-up
```

### 4.4 Smoke-check

```bash
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:8080/ready
curl -sS http://127.0.0.1:8081/health
curl -sS http://127.0.0.1:8081/ready
```

## 5) Вариант B: systemd + локальные сервисы

Если хотите без docker compose:
1. Поднимите PostgreSQL/Redis отдельно.
2. Соберите бинарники:

```bash
GOOS=linux GOARCH=amd64 go build -o /opt/ryazanvpn/bin/control-plane ./cmd/control-plane
GOOS=linux GOARCH=amd64 go build -o /opt/ryazanvpn/bin/node-agent ./cmd/node-agent
```

3. Разделите env на:
- `/etc/ryazanvpn/control-plane.env`
- `/etc/ryazanvpn/node-agent.env`

4. Установите systemd unit'ы из `deploy/systemd/*`.

## 6) Настройка Telegram webhook

После запуска control-plane:

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_BASE_URL}/internal/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

Проверка:

```bash
curl -sS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getWebhookInfo"
```

## 7) Проверка полного цикла

1. В Telegram: `/start`.
2. Админом создайте invite code.
3. Пользователь активирует код.
4. Пользователь получает конфиг по ссылке.
5. Проверьте admin endpoints (секрет в header).
6. Проверьте, что worker трафика пишет агрегаты в `traffic_usage_daily`.

## 8) Минимальные production шаги

- Поставить reverse proxy с TLS (Nginx/Caddy).
- Ограничить внешние порты через UFW:
  - открыть 80/443,
  - закрыть публичный доступ к 5432/6379.
- Делать бэкапы Postgres.
- Включить лог-ротацию.
- Вынести секреты в secret manager (или минимум root-only env files).
