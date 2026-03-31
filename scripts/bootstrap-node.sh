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

echo "Starting node-agent stack..."
docker compose --env-file "$TARGET_ENV" -f docker-compose.node.yml up -d --build

echo "Node bootstrap completed."
