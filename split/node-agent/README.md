# node-agent (standalone repo-ready)

Эта директория самодостаточна: можно скопировать её содержимое в отдельный git-репозиторий node-agent.
Внутри оставлен только entrypoint `cmd/node-agent`.

## Быстрый старт

```bash
cp .env.example .env.generated
# заполните NODE_ID/NODE_TOKEN/CONTROL_PLANE_BASE_URL/AGENT_HMAC_SECRET и runtime поля
make up
```

Сервисы в `docker-compose.yml`:
- `amnezia-awg`
- `xray`
- `node-agent`

## Полезные команды

```bash
make build
make test
make logs
make down
```
