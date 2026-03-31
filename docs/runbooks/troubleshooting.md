# Troubleshooting runbook

## invalid CONFIG_MASTER_KEY

Симптомы:
- `control-plane` не стартует;
- в логах есть `invalid CONFIG_MASTER_KEY` или `invalid key length`.

Что делать:
1. Проверьте, что `CONFIG_MASTER_KEY` — base64-строка от **ровно 32 байт**.
2. Для локального smoke можно использовать пример из env template:
   - `MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=`.
3. Перезапустите `control-plane` после правки env.

## xray unknown command

Симптомы:
- контейнер `xray` падает;
- в логах `xray xray: unknown command`.

Причина:
- в compose передан `command` c лишним `xray` при image, где entrypoint уже `xray`.

Что делать:
1. Использовать command в формате: `run -config /etc/xray/config.json`.
2. Проверить, что используется стабильный image и корректный mount `deploy/node/xray/config.json:/etc/xray/config.json:ro`.

## missing XRAY_CONTAINER_NAME

Симптомы:
- `node-agent` heartbeat/reconcile видит runtime ошибку `xray container is not configured`.

Что делать:
1. Обязательно задайте `XRAY_CONTAINER_NAME` в `.env.node.generated` / `.env.single.generated`.
2. Убедитесь, что значение совпадает с `container_name` сервиса `xray`.

## node-agent cannot exec docker

Симптомы:
- ошибки вида `/usr/bin/docker: no such file or directory`.

Причина:
- node-agent жёстко указывал путь, который отсутствует в конкретном image/host mount wiring.

Что делать:
1. Использовать `DOCKER_BINARY_PATH=docker`.
2. Оставить только mount сокета: `/var/run/docker.sock:/var/run/docker.sock`.
3. Не монтировать host binary `/usr/bin/docker` внутрь контейнера.

## AWG runtime not finding config

Симптомы:
- runtime стартует, но в логах `config not found in /etc/amnezia`;
- интерфейс не получает expected address/peers.

Что делать:
1. Проверить mount `./deploy/node/amnezia:/etc/amnezia`.
2. Убедиться, что есть один из файлов:
   - `${AMNEZIA_INTERFACE_NAME}.conf`
   - `amneziawg.conf`
   - `wg0.conf`
   - `server.conf`
3. Проверить, что в `[Interface]` есть `Address` и валидные ключи.

## kernel-backed AWG expectations

Проверка, что используется kernel-backed path:
1. В логах runtime нет запуска `amneziawg-go`.
2. `docker exec <amnezia_container> awg show <iface>` возвращает интерфейс.
3. `ip link show <iface>` внутри runtime-контейнера показывает поднятый интерфейс.

Важно:
- desktop Amnezia на сервере не требуется;
- runtime должен управляться только через node-agent reconcile.
