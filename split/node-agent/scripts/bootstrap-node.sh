#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET_ENV=".env"
EXAMPLE_ENV="deploy/env/node.env.example"
NON_INTERACTIVE="0"
VALIDATE_ONLY="0"
SKIP_UP="0"
WRITE_ENV="0"
DRY_RUN="0"

log() { echo "[bootstrap-node] $*"; }
fail() { echo "[bootstrap-node] ERROR: $*" >&2; exit 1; }

usage() {
  cat <<USAGE
Usage: scripts/bootstrap-node.sh [options]

Options:
  --env-file <path>      Target env file (default: .env)
  --write-env            Initialize env file from example when missing
  --non-interactive      Do not prompt for missing values, fail instead
  --validate-only        Validate + render only, do not start services
  --skip-up              Skip docker compose up
  --dry-run              Do not write generated configs and do not start services
  -h, --help             Show this help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) shift; [[ $# -gt 0 ]] || fail "--env-file requires value"; TARGET_ENV="$1" ;;
    --write-env) WRITE_ENV="1" ;;
    --non-interactive) NON_INTERACTIVE="1" ;;
    --validate-only) VALIDATE_ONLY="1" ;;
    --skip-up) SKIP_UP="1" ;;
    --dry-run) DRY_RUN="1" ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
  shift
done

if [[ ! -f "$TARGET_ENV" ]]; then
  if [[ "$WRITE_ENV" != "1" ]]; then
    fail "env file not found: $TARGET_ENV (run with --write-env for one-time init)"
  fi
  [[ -f "$EXAMPLE_ENV" ]] || fail "template not found: $EXAMPLE_ENV"
  cp "$EXAMPLE_ENV" "$TARGET_ENV"
  log "Initialized $TARGET_ENV from template (one-time operation)"
fi

if [[ "$NON_INTERACTIVE" != "1" ]]; then
  log "Interactive prompting is deprecated; use explicit env values."
fi

./scripts/config-runtimectl.sh validate-config --env-file "$TARGET_ENV"
if [[ "$DRY_RUN" == "1" ]]; then
  ./scripts/config-runtimectl.sh render-config --env-file "$TARGET_ENV" --dry-run
else
  ./scripts/config-runtimectl.sh render-config --env-file "$TARGET_ENV"
fi

if [[ "$VALIDATE_ONLY" == "1" ]]; then
  log "Validation/render completed (--validate-only)."
  exit 0
fi

if [[ "$SKIP_UP" == "1" || "${BOOTSTRAP_SKIP_DOCKER:-0}" == "1" || "$DRY_RUN" == "1" ]]; then
  log "Skipping docker compose up (skip flag or dry-run)."
  exit 0
fi

log "Starting node stack..."
./scripts/compose-with-env.sh "$TARGET_ENV" -f docker-compose.yml up -d --build
log "Bootstrap complete."
