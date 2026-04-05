# Monorepo split exports

Эта папка содержит две готовые директории для вынесения в отдельные репозитории:

- `control-plane/`
- `node-agent/`

## Как использовать

1. Скопируйте содержимое `split/control-plane` в новый репозиторий `ryazanvpn-control-plane`.
2. Скопируйте содержимое `split/node-agent` в новый репозиторий `ryazanvpn-node-agent`.
3. В каждом репозитории:
   - `cp .env.example .env.generated`
   - заполните `.env.generated`
   - `make up`
