# control-plane (standalone repo-ready)

Эта директория самодостаточна: можно скопировать её содержимое в отдельный git-репозиторий control-plane.

## Быстрый старт

```bash
cp .env.example .env.generated
# заполните секреты и URL в .env.generated
make up
```

Сервисы в `docker-compose.yml`:
- `postgres`
- `redis`
- `migrate`
- `control-plane`

## Полезные команды

```bash
make build
make test
make logs
make down
```
