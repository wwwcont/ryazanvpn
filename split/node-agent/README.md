# node-agent (standalone repo-ready)

## Новый конфигурационный контракт (source of truth)

- `./.env` — **единственный desired source of truth** для node runtime-параметров.
- `deploy/env/node.env.example` — только шаблон для первичной инициализации (`bootstrap-node.sh --write-env`).
- `deploy/node/amnezia/awg0.conf` и `deploy/node/xray/config.json` — детерминированные generated-артефакты, рендерятся только из `.env`.
- runtime (`docker exec`, `awg show`, `docker inspect`) — только диагностика/drift detection/reconcile, никогда не пишет обратно в `.env`.

## Команды

```bash
# 1) Однократная инициализация .env
./scripts/bootstrap-node.sh --write-env --env-file .env --validate-only

# 2) Проверка desired config
./scripts/config-runtimectl.sh validate-config --env-file .env

# 3) Рендер generated-конфигов
./scripts/config-runtimectl.sh render-config --env-file .env

# 4) Диагностика runtime drift (exit 1 при drift)
./scripts/config-runtimectl.sh inspect-runtime --env-file .env

# 5) Reconcile runtime из desired config
./scripts/config-runtimectl.sh reconcile-runtime --env-file .env
```

## Важно

- Скрипт `sync-runtime-from-configs.sh` deprecated и переведен в read-only режим.
- Скрытые перезаписи `.env` удалены.
- Для всех операций поддержан `--dry-run` в `config-runtimectl.sh`.
