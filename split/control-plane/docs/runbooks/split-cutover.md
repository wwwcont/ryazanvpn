# Repo split cutover runbook

Цель: безопасно перейти с monorepo на 2 репозитория (`control-plane`, `node-agent`) без остановки разработки и без поломки протокола.

## 1) Freeze window (границы импортов)

Перед cutover объявляется freeze-window:

- новые cross-service импорты запрещены;
- изменения в контрактах (`shared/contracts/nodeapi`) проходят через отдельный review;
- merge только после зелёного service CI + boundary checks.

## 2) Release train (порядок релиза)

Порядок строго такой:

1. `shared/contracts` (контракты, версия);
2. `control-plane`;
3. `node-agent`.

Совместимость на этапе cutover: `N/N-1` по `X-Protocol-Version`.

## 3) Canary rollout node-agent

1. Выбрать 5–10 нод (разные регионы/типы нагрузки).
2. Обновить только canary-группу.
3. Выдержать окно наблюдения (минимум 2 часа, лучше 24 часа).
4. Проверить метрики/логи:
   - ошибки `unsupported protocol version`,
   - доля 4xx/5xx на `/nodes/*`,
   - ошибки reconcile/apply/traffic.

Если canary зелёный — продолжить staged rollout (например, 25% -> 50% -> 100%).

## 4) Rollback

Rollback должен быть определён для каждого шага:

- contracts: вернуть предыдущую контрактную версию и теги;
- control-plane: откат на предыдущий образ/релиз;
- node-agent: откат canary-группы и остановка дальнейшего rollout.

Правило: при ошибках протокольной совместимости rollout немедленно останавливается.

## 5) Smoke-checklist после каждого этапа

- `GET /health` control-plane — 200;
- `GET /health` node-agent на canary нодах — 200;
- heartbeat/desired/apply проходят без всплеска ошибок;
- нет роста `unsupported protocol version`;
- ключевые пользовательские сценарии (создание устройства, выдача конфига) успешны.
