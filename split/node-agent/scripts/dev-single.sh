#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${SINGLE_ENV:-.env}"
COMPOSE=(./scripts/compose-with-env.sh "$ENV_FILE" -f docker-compose.single.yml)

usage() {
  cat <<USAGE
Usage: $0 <command>

Commands:
  up       Sync runtime values from configs and start app stack
  rebuild  Sync runtime values and rebuild/restart app stack
  sync     Only sync runtime values from config paths in env
  down     Stop app stack
  logs     Follow logs for whole stack
  ps       Show container status
USAGE
}

cmd="${1:-}"
case "$cmd" in
  up)
    ./scripts/sync-runtime-from-configs.sh "$ENV_FILE"
    "${COMPOSE[@]}" up -d --build
    ;;
  rebuild)
    ./scripts/sync-runtime-from-configs.sh "$ENV_FILE"
    "${COMPOSE[@]}" up -d --build --force-recreate
    ;;
  sync)
    ./scripts/sync-runtime-from-configs.sh "$ENV_FILE"
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  logs)
    "${COMPOSE[@]}" logs -f
    ;;
  ps)
    "${COMPOSE[@]}" ps
    ;;
  *)
    usage
    exit 1
    ;;
esac
