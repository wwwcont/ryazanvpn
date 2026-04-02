# Runbook: подключение дополнительной ноды к главной ноде (control-plane)

## Предпосылки

- Главная нода с `control-plane` уже работает.
- У вас есть значения:
  - `NODE_ID`
  - `NODE_TOKEN`
  - `CONTROL_PLANE_BASE_URL`
  - `AGENT_HMAC_SECRET`

## Этап 1. Подготовка новой VPS

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

## Этап 3. Bootstrap

```bash
./scripts/bootstrap-node.sh
```

Скрипт создаст `.env.node.generated` и runtime-директории.

## Этап 4. Настройка env

Откройте `.env.node.generated` и заполните:
- `NODE_ID`
- `NODE_TOKEN`
- `CONTROL_PLANE_BASE_URL`
- `AGENT_HMAC_SECRET`
- `NODE_NAME`
- `NODE_REGION`
- `NODE_PUBLIC_IP`
- `AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`, `XRAY_CONTAINER_NAME`
- `AMNEZIA_CONFIG_PATH`, `AMNEZIA_PUBLIC_KEY_SOURCE_PATH`, `XRAY_SOURCE_CONFIG_PATH`, `XRAY_REALITY_PUBLIC_KEY_SOURCE_PATH`

## Этап 5. Запуск node stack

```bash
docker compose --env-file .env.node.generated -f docker-compose.node.yml up -d --build
```

## Этап 6. Проверка

```bash
docker compose --env-file .env.node.generated -f docker-compose.node.yml ps
curl http://localhost:8081/health
curl http://localhost:8081/ready
```

Проверьте на control-plane, что нода появилась в метриках/админке.

## Важно

- Не используйте desktop Amnezia GUI на сервере.
- Не правьте peers/clients руками внутри контейнеров.
- Источник правды — control-plane desired state.
