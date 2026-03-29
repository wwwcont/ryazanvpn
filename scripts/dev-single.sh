#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${SINGLE_ENV:-.env.single.generated}"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f docker-compose.single.yml)

usage() {
  cat <<USAGE
Usage: $0 <command>

Commands:
  up       Start single-server stack in detached mode
  down     Stop single-server stack
  rebuild  Rebuild and restart stack in detached mode
  logs     Follow logs for whole stack (manual)
  ps       Show container status
USAGE
}

cmd="${1:-}"
case "$cmd" in
  up)
    "${COMPOSE[@]}" up -d --build
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  rebuild)
    "${COMPOSE[@]}" up -d --build --force-recreate
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
