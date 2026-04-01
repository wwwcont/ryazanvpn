#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET_ENV=".env.node.generated"
EXAMPLE_ENV="deploy/env/node.env.example"

if [[ ! -f "$TARGET_ENV" ]]; then
  cp "$EXAMPLE_ENV" "$TARGET_ENV"
  echo "Created $TARGET_ENV from template. Fill NODE_ID / NODE_TOKEN / CONTROL_PLANE_BASE_URL."
fi
set -a
source "$TARGET_ENV"
set +a

mkdir -p deploy/node/amnezia deploy/node/xray
if [[ ! -f deploy/node/xray/config.json ]]; then
  echo "deploy/node/xray/config.json is missing. Create it from repository template before start."
  exit 1
fi

./scripts/ensure-amnezia-server-config.sh "$TARGET_ENV" "deploy/node/amnezia/awg0.conf"
./scripts/ensure-xray-server-config.sh "$TARGET_ENV" "deploy/node/xray/config.json"

echo "Starting node-agent stack..."
./scripts/compose-with-env.sh "$TARGET_ENV" -f docker-compose.node.yml up -d --build

if [[ -n "${AMNEZIA_CONTAINER_NAME:-}" ]] && [[ -n "${AMNEZIA_INTERFACE_NAME:-}" ]] && [[ -n "${AMNEZIA_PORT:-}" ]]; then
  actual_port="$(docker exec "${AMNEZIA_CONTAINER_NAME}" awg show "${AMNEZIA_INTERFACE_NAME}" listen-port 2>/dev/null || true)"
  echo "amnezia.server_listen_port.expected value=${AMNEZIA_PORT}" >&2
  echo "amnezia.server_listen_port.actual value=${actual_port:-unknown}" >&2
  if [[ "${actual_port}" != "${AMNEZIA_PORT}" ]]; then
    echo "amnezia.server_listen_port.mismatch expected=${AMNEZIA_PORT} actual=${actual_port:-unknown}" >&2
    exit 1
  fi
fi

echo "Node bootstrap completed."
echo "IMPORTANT: runtime is fully managed by node-agent. Do NOT use desktop Amnezia GUI on this host."
