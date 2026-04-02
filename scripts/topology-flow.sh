#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.single.generated}"
TOPOLOGY_MODE="${TOPOLOGY_MODE:-single-node}"
INSTALL_ROLE="${INSTALL_ROLE:-all}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.topology.yml}"
export ENV_FILE_PATH="$ENV_FILE"
COMPOSE=(./scripts/compose-with-env.sh "$ENV_FILE" -f "$COMPOSE_FILE")

usage() {
  cat <<USAGE
Usage: $0 <command>

Commands:
  runtime-up   Start VPN runtime layer (amnezia-awg, xray)
  sync-env     Sync runtime metadata/public keys to env file
  control-up   Start control-plane layer (postgres, redis, migrate, control-plane)
  node-up      Start node layer (node-agent)
  ps           Show compose status
  logs         Follow logs
  down         Stop stack

Env:
  ENV_FILE=.env.single.generated
  TOPOLOGY_MODE=single-node|control-plane-only|node-only|distributed
  INSTALL_ROLE=all|control-plane|node   (used when TOPOLOGY_MODE=distributed)
USAGE
}

is_allowed() {
  local action="$1"
  case "$TOPOLOGY_MODE" in
    single-node)
      return 0
      ;;
    control-plane-only)
      [[ "$action" == "control" || "$action" == "obs" ]]
      ;;
    node-only)
      [[ "$action" == "runtime" || "$action" == "sync" || "$action" == "node" || "$action" == "obs" ]]
      ;;
    distributed)
      case "$INSTALL_ROLE" in
        all) [[ "$action" == "runtime" || "$action" == "sync" || "$action" == "control" || "$action" == "node" || "$action" == "obs" ]] ;;
        control-plane) [[ "$action" == "control" || "$action" == "obs" ]] ;;
        node) [[ "$action" == "runtime" || "$action" == "sync" || "$action" == "node" || "$action" == "obs" ]] ;;
        *) echo "unsupported INSTALL_ROLE=$INSTALL_ROLE" >&2; return 1 ;;
      esac
      ;;
    *)
      echo "unsupported TOPOLOGY_MODE=$TOPOLOGY_MODE" >&2
      return 1
      ;;
  esac
}

cmd="${1:-}"
case "$cmd" in
  runtime-up)
    is_allowed runtime || { echo "runtime-up is not allowed for mode=$TOPOLOGY_MODE role=$INSTALL_ROLE" >&2; exit 1; }
    "${COMPOSE[@]}" up -d --build amnezia-awg xray
    ;;
  sync-env)
    is_allowed sync || { echo "sync-env is not allowed for mode=$TOPOLOGY_MODE role=$INSTALL_ROLE" >&2; exit 1; }
    ./scripts/runtime-sync-env.sh "$ENV_FILE"
    ;;
  control-up)
    is_allowed control || { echo "control-up is not allowed for mode=$TOPOLOGY_MODE role=$INSTALL_ROLE" >&2; exit 1; }
    "${COMPOSE[@]}" up -d --build postgres redis migrate control-plane
    ;;
  node-up)
    is_allowed node || { echo "node-up is not allowed for mode=$TOPOLOGY_MODE role=$INSTALL_ROLE" >&2; exit 1; }
    "${COMPOSE[@]}" up -d --build node-agent
    ;;
  ps)
    is_allowed obs || { exit 1; }
    "${COMPOSE[@]}" ps
    ;;
  logs)
    is_allowed obs || { exit 1; }
    "${COMPOSE[@]}" logs -f
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  *)
    usage
    exit 1
    ;;
esac
