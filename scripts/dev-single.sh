#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${SINGLE_ENV:-.env.single.generated}"
COMPOSE=(./scripts/compose-with-env.sh "$ENV_FILE" -f docker-compose.single.yml)

usage() {
  cat <<USAGE
Usage: $0 <command>

Commands:
  up       Start single-server stack in detached mode
  vpn-up   Start only VPN runtime containers (amnezia-awg, xray)
  sync     Sync runtime-generated VPN/Xray values back into env file
  core-up  Start control-plane core (postgres, redis, migrate, control-plane)
  node-up  Start node-agent after control-plane is ready
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
  vpn-up)
    ENV_FILE="$ENV_FILE" TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh runtime-up
    ;;
  sync)
    ./scripts/runtime-sync-env.sh "$ENV_FILE"
    ;;
  core-up)
    ENV_FILE="$ENV_FILE" TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh control-up
    ;;
  node-up)
    ENV_FILE="$ENV_FILE" TOPOLOGY_MODE=single-node ./scripts/topology-flow.sh node-up
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
