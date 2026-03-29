# Runbook: single-server deploy

1. `cp deploy/env/single-server.env.example .env.single.generated` and fill secrets.
2. `make run-single`.
3. Check: `/health`, `/ready`, `/internal/telegram/webhook`.
4. Configure Telegram webhook to `${PUBLIC_BASE_URL}/internal/telegram/webhook`.
