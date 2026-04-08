#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env}"

echo "[sync-runtime-from-configs] DEPRECATED: script is read-only now and does not mutate .env" >&2
./scripts/config-runtimectl.sh inspect-runtime --env-file "$ENV_FILE"
