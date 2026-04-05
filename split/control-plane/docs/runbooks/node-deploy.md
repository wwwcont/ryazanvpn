# Runbook: поднять ноду за 5–10 минут

## One-command bootstrap

```bash
./scripts/bootstrap-node.sh
```

Скрипт делает wizard/bootstrap:
- автосчитывает доступные параметры из `deploy/node/amnezia/awg0.conf` и `deploy/node/xray/config.json`,
- спрашивает только недостающие секреты (`NODE_TOKEN`, `AGENT_HMAC_SECRET`, `XRAY_REALITY_PRIVATE_KEY`),
- валидирует обязательные env и preflight-зависимости,
- генерирует/обновляет `.env.node.generated`,
- запускает `docker compose` node-стека.

## Preflight only (без запуска контейнеров)

```bash
BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --validate-only
```

## Non-interactive mode (для CI/автоматизации)

```bash
BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --env-file .env.node.generated --non-interactive --validate-only
```

Если обязательные параметры не заданы, скрипт завершится с human-readable ошибкой.

## Чеклист после запуска

```bash
docker compose --env-file .env.node.generated -f docker-compose.node.yml ps
curl -fsS http://localhost:8081/health
curl -fsS http://localhost:8081/ready
```

Node-agent автоматически регистрирует ноду в control-plane по `NODE_ID`/`NODE_TOKEN`.
