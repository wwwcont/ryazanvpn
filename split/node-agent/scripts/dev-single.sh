#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${SINGLE_ENV:-.env}"
COMPOSE=(./scripts/compose-with-env.sh "$ENV_FILE" -f docker-compose.single.yml)

usage() {
  cat <<USAGE
Usage: $0 <command>

Commands:
  up       Validate, render, reconcile runtime and start app stack
  rebuild  Validate, render, reconcile runtime and rebuild/restart app stack
  validate Validate desired config only
  render   Render generated config only
  inspect  Inspect runtime drift only (exit non-zero on drift)
  reconcile Reconcile runtime from desired config (never mutates .env)
  down     Stop app stack
  logs     Follow logs for whole stack
  ps       Show container status
USAGE
}

cmd="${1:-}"
case "$cmd" in
  up)
    ./scripts/config-runtimectl.sh validate-config --env-file "$ENV_FILE"
    ./scripts/config-runtimectl.sh render-config --env-file "$ENV_FILE"
    ./scripts/config-runtimectl.sh reconcile-runtime --env-file "$ENV_FILE"
    "${COMPOSE[@]}" up -d --build
    ;;
  rebuild)
    ./scripts/config-runtimectl.sh validate-config --env-file "$ENV_FILE"
    ./scripts/config-runtimectl.sh render-config --env-file "$ENV_FILE"
    ./scripts/config-runtimectl.sh reconcile-runtime --env-file "$ENV_FILE"
    "${COMPOSE[@]}" up -d --build --force-recreate
    ;;
  validate)
    ./scripts/config-runtimectl.sh validate-config --env-file "$ENV_FILE"
    ;;
  render)
    ./scripts/config-runtimectl.sh render-config --env-file "$ENV_FILE"
    ;;
  inspect)
    ./scripts/config-runtimectl.sh inspect-runtime --env-file "$ENV_FILE"
    ;;
  reconcile)
    ./scripts/config-runtimectl.sh reconcile-runtime --env-file "$ENV_FILE"
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
