# Runbook: rotate Telegram token

1. Create new token in @BotFather.
2. Update `TELEGRAM_BOT_TOKEN` in deployed env file.
3. Restart `control-plane`.
4. Re-register webhook with new token and same secret.
