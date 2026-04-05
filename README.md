# RyazanVPN — production-minded MVP monorepo

Сервисы:
- `cmd/control-plane`
- `cmd/node-agent`

## Канонический production flow

1. Вы **сами** поднимаете и обслуживаете runtime контейнеры Amnezia/Xray.
2. В `.env.single.generated` указываете runtime-параметры контейнера Amnezia (`AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`) и путь к Xray config (`XRAY_SOURCE_CONFIG_PATH`).
3. Запускаете **одну команду**:

```bash
make single
```

Что делает `make single`:
- читает runtime-файлы Amnezia/Xray и обновляет env (`scripts/sync-runtime-from-configs.sh`);
- поднимает только app stack: `postgres`, `redis`, `migrate`, `control-plane`, `node-agent`.

## Быстрый старт

```bash
cp deploy/env/single-server.env.example .env.single.generated
# заполните секреты + container names + source paths
make single
```

Проверки:

```bash
make ps-single
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/health
```

## Откуда берутся runtime-данные

`scripts/sync-runtime-from-configs.sh` подтягивает:
- из runtime Amnezia через `docker exec ... awg show`: `VPN_SERVER_PUBLIC_KEY`, `AMNEZIA_PORT`;
- из runtime Amnezia через `ip addr show`: `VPN_SUBNET_CIDR` (если доступно);
- из `XRAY_SOURCE_CONFIG_PATH`: `XRAY_REALITY_PORT`, `XRAY_REALITY_SERVER_NAME`, `XRAY_REALITY_SHORT_ID`;
- из env: `XRAY_REALITY_PRIVATE_KEY`, `XRAY_REALITY_PUBLIC_KEY` (единый источник ключей);
- из `VPN_PUBLIC_HOST` + `AMNEZIA_PORT`: `VPN_SERVER_PUBLIC_ENDPOINT`.

`scripts/sync-runtime-from-configs.sh` валидирует, что `privateKey` в runtime Xray config совпадает с `XRAY_REALITY_PRIVATE_KEY`; при рассинхроне завершает работу с ошибкой.

## Runtime logic (не меняли)

Сохранена рабочая логика:
- add/remove peer в Amnezia через `node-agent`;
- модификация Xray config и добавление клиента;
- выдача/скачивание клиентских конфигов через `control-plane`;
- Telegram pipeline выдачи конфига.

## Build/Test

```bash
go test ./...
```

## Billing model (MVP)

- Биллинг считается **по устройствам**: `10 ₽/сутки` за каждое активное устройство пользователя.
- Устройство считается биллингуемым, если у него есть хотя бы один `device_access` в статусе `active` или `suspended_nonpayment`.
- Если активных устройств нет — суточного списания нет.
- Активация invite-кода начисляет бонус `+250 ₽` (`25_000` копеек).

## Device lifecycle и nonpayment suspend/resume

- Поддерживаемые статусы `device_access`: `active`, `suspended_nonpayment`, `revoked` (+ технические `pending/error`).
- При балансе `<= 0` пользователь переводится в nonpayment-состояние, активные доступы уходят в `suspended_nonpayment`.
- При пополнении баланса `> 0` доступы автоматически возвращаются в `active`.
- Suspend/resume выполняется без потери device/config metadata: reconcile/runtime возвращает peer’ы без полного перевыпуска устройства.

## Telegram bot: команды и меню

Поддерживаемые команды:
- `/start`
- `/menu`
- `/balance`
- `/devices`
- `/config`
- `/help`

Главное меню пользователя:
- Мой доступ
- Мои устройства
- Баланс
- Пополнить / активировать код
- Получить конфиг
- Помощь

Карточка устройства:
- статус/дата создания/активные протоколы/участие в биллинге
- перевыпуск WG/Xray конфига (без удаления устройства)
- скачать WG/Xray
- удалить устройство (ревокает все access и останавливает списание по устройству)

### Что выставить в BotFather

1. `/setcommands` — указать список команд выше.
2. `/setdescription` — кратко: VPN-сервис с WireGuard/AmneziaWG и Xray Reality.
3. `/setabouttext` — краткий onboarding + ежедневный биллинг.
4. `/setmenubutton` — открыть web-app/commands (в зависимости от вашей схемы).

## Metrics (admin/dashboard)

Рекомендуемые метрики на дашборд:
- Node: active users/devices, traffic 24h, current rx/tx rate, reconcile success/error.
- User: today/7d/30d трафик, last activity, access status, оценка days remaining по балансу.
- Для расчёта утилизации канала используйте `NODE_LINK_CAPACITY_BPS` (например, `1000000000` для 1 Gbit/s).
- Для server-observed speed используется выборка `node_throughput_samples`:
  - `current_bps_estimate`
  - `median_bps_1h`
  - `p95_bps_1h`
  - `peak_bps_24h`
- Ограничение хранения/шага выборки:
  - `NODE_THROUGHPUT_SAMPLE_STEP` (по умолчанию `1m`)
  - `NODE_THROUGHPUT_RETENTION` (по умолчанию `48h`)
