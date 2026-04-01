#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET_ENV=".env.backend.generated"
EXAMPLE_ENV="deploy/env/backend.env.example"

if [[ ! -f "$TARGET_ENV" ]]; then
  cp "$EXAMPLE_ENV" "$TARGET_ENV"
  echo "Created $TARGET_ENV from template. Please edit secrets before production use."
fi

echo "Starting backend stack (postgres, redis, control-plane, caddy)..."
./scripts/compose-with-env.sh "$TARGET_ENV" -f docker-compose.backend.yml up -d --build

echo "Backend bootstrap completed."
