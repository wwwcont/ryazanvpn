#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${1:-.env}"
CONF_PATH="${2:-deploy/node/amnezia/awg0.conf}"

[[ -f "$ENV_FILE" ]] || { echo "amnezia.server_config.write.error env_file=$ENV_FILE reason=env_file_not_found" >&2; exit 1; }

while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
  line="${raw_line#"${raw_line%%[![:space:]]*}"}"
  [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
  [[ "$line" != *"="* ]] && continue
  key="${line%%=*}"; value="${line#*=}"
  printf -v "$key" '%s' "$value"
  export "$key"
done < "$ENV_FILE"

iface="${AMNEZIA_INTERFACE_NAME:-awg0}"
expected_port="${AMNEZIA_PORT:-${AMNEZIA_LISTEN_PORT:-}}"
private_key="${AMNEZIA_PRIVATE_KEY:-}"

[[ -n "$expected_port" ]] || { echo "amnezia.server_config.write.error path=$CONF_PATH iface=$iface reason=amnezia_port_missing" >&2; exit 1; }
[[ -n "$private_key" ]] || { echo "amnezia.server_config.write.error path=$CONF_PATH iface=$iface reason=private_key_missing" >&2; exit 1; }

mkdir -p "$(dirname "$CONF_PATH")"
echo "amnezia.server_config.write.start path=$CONF_PATH iface=$iface expected_port=$expected_port" >&2

tmp="$(mktemp)"
{
  echo "[Interface]"
  echo "PrivateKey = $private_key"
  echo "ListenPort = $expected_port"
} > "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$CONF_PATH"

echo "amnezia.server_config.write.success path=$CONF_PATH iface=$iface listen_port=$expected_port" >&2
