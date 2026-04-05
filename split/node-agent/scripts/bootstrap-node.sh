#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET_ENV=".env"
EXAMPLE_ENV="deploy/env/node.env.example"
AMNEZIA_CONF="deploy/node/amnezia/awg0.conf"
XRAY_CONF="deploy/node/xray/config.json"

NON_INTERACTIVE="0"
VALIDATE_ONLY="0"
SKIP_UP="0"

log() { echo "[bootstrap-node] $*"; }
warn() { echo "[bootstrap-node] WARN: $*" >&2; }
fail() { echo "[bootstrap-node] ERROR: $*" >&2; exit 1; }

usage() {
  cat <<USAGE
Usage: scripts/bootstrap-node.sh [options]

Options:
  --env-file <path>      Target env file (default: .env)
  --non-interactive      Do not prompt for missing values, fail instead
  --validate-only        Run preflight+validation only, do not start services
  --skip-up              Skip docker compose up (useful for tests)
  -h, --help             Show this help

One-command quick start:
  scripts/bootstrap-node.sh
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file)
      shift
      [[ $# -gt 0 ]] || fail "--env-file requires value"
      TARGET_ENV="$1"
      ;;
    --non-interactive)
      NON_INTERACTIVE="1"
      ;;
    --validate-only)
      VALIDATE_ONLY="1"
      ;;
    --skip-up)
      SKIP_UP="1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1"
      ;;
  esac
  shift
done

trim() {
  local v="$1"
  v="${v#${v%%[![:space:]]*}}"
  v="${v%${v##*[![:space:]]}}"
  printf '%s' "$v"
}

set_env_kv() {
  local file="$1" key="$2" value="$3"
  if grep -qE "^${key}=" "$file"; then
    sed -i "s|^${key}=.*|${key}=${value}|" "$file"
  else
    printf "%s=%s\n" "$key" "$value" >> "$file"
  fi
}

load_env_file() {
  local file="$1"
  [[ -f "$file" ]] || fail "env file not found: $file"
  while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
    local line key value
    line="$(trim "$raw_line")"
    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" == *"="* ]] || continue
    key="${line%%=*}"
    value="${line#*=}"
    printf -v "$key" '%s' "$value"
    export "$key"
  done < "$file"
}

prompt_if_missing() {
  local key="$1" label="$2" secret="${3:-0}" current=""
  current="${!key:-}"
  current="$(trim "$current")"
  if [[ -n "$current" ]]; then
    return 0
  fi
  if [[ "$NON_INTERACTIVE" == "1" ]]; then
    fail "missing required value: $key ($label). Run without --non-interactive to fill it."
  fi
  if [[ "$secret" == "1" ]]; then
    read -r -s -p "$label: " current
    echo
  else
    read -r -p "$label: " current
  fi
  current="$(trim "$current")"
  [[ -n "$current" ]] || fail "empty value is not allowed for $key"
  export "$key=$current"
}

auto_detect_amnezia() {
  [[ -f "$AMNEZIA_CONF" ]] || return 0
  local detected_priv detected_port
  detected_priv="$(awk -F'=' '/^[[:space:]]*PrivateKey[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}' "$AMNEZIA_CONF")"
  detected_port="$(awk -F'=' '/^[[:space:]]*ListenPort[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}' "$AMNEZIA_CONF")"
  if [[ -n "$detected_priv" && -z "${AMNEZIA_PRIVATE_KEY:-}" ]]; then
    export AMNEZIA_PRIVATE_KEY="$detected_priv"
    log "Detected AMNEZIA_PRIVATE_KEY from $AMNEZIA_CONF"
  fi
  if [[ -n "$detected_port" && -z "${AMNEZIA_PORT:-}" ]]; then
    export AMNEZIA_PORT="$detected_port"
    log "Detected AMNEZIA_PORT=$detected_port from $AMNEZIA_CONF"
  fi
}

auto_detect_xray() {
  [[ -f "$XRAY_CONF" ]] || return 0
  local out
  out="$(python3 - "$XRAY_CONF" <<'PY'
import json,sys
p=sys.argv[1]
obj=json.load(open(p,'r',encoding='utf-8'))
inb=(obj.get('inbounds') or [{}])[0]
stream=inb.get('streamSettings') or {}
reality=stream.get('realitySettings') or {}
private_key=reality.get('privateKey') or ''
port=str(inb.get('port') or '')
sni=''
if isinstance(reality.get('serverNames'), list) and reality['serverNames']:
    sni=str(reality['serverNames'][0])
short_id=''
if isinstance(reality.get('shortIds'), list) and reality['shortIds']:
    short_id=str(reality['shortIds'][0])
print(private_key)
print(port)
print(sni)
print(short_id)
PY
)"
  local detected_priv detected_port detected_sni detected_short
  detected_priv="$(echo "$out" | sed -n '1p')"
  detected_port="$(echo "$out" | sed -n '2p')"
  detected_sni="$(echo "$out" | sed -n '3p')"
  detected_short="$(echo "$out" | sed -n '4p')"

  if [[ -n "$detected_priv" && -z "${XRAY_REALITY_PRIVATE_KEY:-}" ]]; then
    export XRAY_REALITY_PRIVATE_KEY="$detected_priv"
    log "Detected XRAY_REALITY_PRIVATE_KEY from $XRAY_CONF"
  fi
  if [[ -n "$detected_port" && -z "${XRAY_REALITY_PORT:-}" ]]; then
    export XRAY_REALITY_PORT="$detected_port"
  fi
  if [[ -n "$detected_sni" && -z "${XRAY_REALITY_SERVER_NAME:-}" ]]; then
    export XRAY_REALITY_SERVER_NAME="$detected_sni"
  fi
  if [[ -n "$detected_short" && -z "${XRAY_REALITY_SHORT_ID:-}" ]]; then
    export XRAY_REALITY_SHORT_ID="$detected_short"
  fi
}

validate_env() {
  local missing=0
  for key in NODE_ID NODE_TOKEN CONTROL_PLANE_BASE_URL AGENT_HMAC_SECRET AMNEZIA_CONTAINER_NAME AMNEZIA_INTERFACE_NAME XRAY_CONTAINER_NAME XRAY_REALITY_PRIVATE_KEY; do
    if [[ -z "$(trim "${!key:-}")" ]]; then
      warn "required env is missing: $key"
      missing=1
    fi
  done
  [[ -f "$XRAY_CONF" ]] || { warn "missing file: $XRAY_CONF"; missing=1; }
  if [[ "$missing" -ne 0 ]]; then
    fail "preflight failed: required configuration is incomplete"
  fi
}

preflight_tools() {
  command -v python3 >/dev/null 2>&1 || fail "python3 is required for xray config validation"
  command -v awk >/dev/null 2>&1 || fail "awk is required"
  if [[ "${BOOTSTRAP_SKIP_DOCKER:-0}" == "1" ]]; then
    warn "BOOTSTRAP_SKIP_DOCKER=1 -> docker runtime checks skipped"
    return
  fi
  command -v docker >/dev/null 2>&1 || fail "docker is required"
  docker info >/dev/null 2>&1 || fail "docker daemon is unavailable (start docker and retry)"
}

write_generated_env() {
  [[ -f "$TARGET_ENV" ]] || cp "$EXAMPLE_ENV" "$TARGET_ENV"
  set_env_kv "$TARGET_ENV" "NODE_ID" "${NODE_ID}"
  set_env_kv "$TARGET_ENV" "NODE_TOKEN" "${NODE_TOKEN}"
  set_env_kv "$TARGET_ENV" "CONTROL_PLANE_BASE_URL" "${CONTROL_PLANE_BASE_URL}"
  set_env_kv "$TARGET_ENV" "AGENT_HMAC_SECRET" "${AGENT_HMAC_SECRET}"
  set_env_kv "$TARGET_ENV" "NODE_AGENT_HMAC_SECRET" "${AGENT_HMAC_SECRET}"
  set_env_kv "$TARGET_ENV" "AMNEZIA_PORT" "${AMNEZIA_PORT:-41475}"
  set_env_kv "$TARGET_ENV" "XRAY_REALITY_PRIVATE_KEY" "${XRAY_REALITY_PRIVATE_KEY}"
  if [[ -n "${XRAY_REALITY_PORT:-}" ]]; then
    set_env_kv "$TARGET_ENV" "XRAY_REALITY_PORT" "${XRAY_REALITY_PORT}"
  fi
  if [[ -n "${XRAY_REALITY_SERVER_NAME:-}" ]]; then
    set_env_kv "$TARGET_ENV" "XRAY_REALITY_SERVER_NAME" "${XRAY_REALITY_SERVER_NAME}"
  fi
  if [[ -n "${XRAY_REALITY_SHORT_ID:-}" ]]; then
    set_env_kv "$TARGET_ENV" "XRAY_REALITY_SHORT_ID" "${XRAY_REALITY_SHORT_ID}"
  fi
  log "Generated/updated env file: $TARGET_ENV"
}

main() {
  [[ -f "$EXAMPLE_ENV" ]] || fail "template not found: $EXAMPLE_ENV"
  mkdir -p deploy/node/amnezia deploy/node/xray

  if [[ ! -f "$TARGET_ENV" ]]; then
    cp "$EXAMPLE_ENV" "$TARGET_ENV"
    log "Created $TARGET_ENV from template"
  fi

  load_env_file "$TARGET_ENV"
  auto_detect_amnezia
  auto_detect_xray

  prompt_if_missing NODE_ID "Node ID"
  prompt_if_missing NODE_TOKEN "Node Token" 1
  prompt_if_missing CONTROL_PLANE_BASE_URL "Control-plane base URL (e.g. http://control-plane:8080)"
  prompt_if_missing AGENT_HMAC_SECRET "Shared HMAC secret for node-agent" 1
  prompt_if_missing XRAY_REALITY_PRIVATE_KEY "Xray reality private key" 1

  write_generated_env
  load_env_file "$TARGET_ENV"

  preflight_tools
  validate_env

  ./scripts/ensure-amnezia-server-config.sh "$TARGET_ENV" "$AMNEZIA_CONF"
  ./scripts/ensure-xray-server-config.sh "$TARGET_ENV" "$XRAY_CONF"

  if [[ "$VALIDATE_ONLY" == "1" ]]; then
    log "Preflight and config validation completed successfully (--validate-only)."
    exit 0
  fi

  if [[ "$SKIP_UP" == "1" || "${BOOTSTRAP_SKIP_DOCKER:-0}" == "1" ]]; then
    log "Skipping docker compose up (SKIP_UP/BOOTSTRAP_SKIP_DOCKER set)."
    exit 0
  fi

  log "Starting node stack and auto-registration loop..."
  ./scripts/compose-with-env.sh "$TARGET_ENV" -f docker-compose.yml up -d --build

  log "Bootstrap complete: node-agent is running and will self-register in control-plane using NODE_ID/NODE_TOKEN."
  log "Quick checks:"
  echo "  curl -fsS http://localhost:8081/health"
  echo "  curl -fsS http://localhost:8081/ready"
}

main "$@"
