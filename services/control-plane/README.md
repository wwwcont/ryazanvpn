# Control-plane service

`services/control-plane` — папка запуска и эксплуатации control-plane как отдельного сервиса.

## Что находится в папке

- `Makefile` — build/test/up/down для control-plane.
- `.env.example` — пример переменных окружения только для control-plane стека.
- `docker-compose.yml` — compose-стек control-plane (postgres/redis/migrate/control-plane/caddy).

## Быстрый запуск

```bash
cp services/control-plane/.env.example services/control-plane/.env.generated
make -C services/control-plane up
```

Остановка:

```bash
make -C services/control-plane down
```

## Проверки

```bash
curl -fsS http://localhost:8080/health
```

## Что настраивать обязательно

- `POSTGRES_*`, `POSTGRES_URL`
- `REDIS_*`
- `AGENT_HMAC_SECRET` / `NODE_AGENT_HMAC_SECRET`
- `ADMIN_API_SECRET`
- `CONFIG_MASTER_KEY`
- `NODE_REGISTRATION_TOKEN`
- `PUBLIC_BASE_URL` и (если нужен HTTPS) `PUBLIC_DOMAIN`
