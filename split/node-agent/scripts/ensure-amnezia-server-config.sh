#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${1:-.env}"
CONF_PATH="${2:-deploy/node/amnezia/awg0.conf}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "amnezia.server_config.write.error env_file=$ENV_FILE reason=env_file_not_found" >&2
  exit 1
fi

while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
  line="${raw_line#"${raw_line%%[![:space:]]*}"}"
  [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
  [[ "$line" != *"="* ]] && continue
  key="${line%%=*}"
  value="${line#*=}"
  printf -v "$key" '%s' "$value"
  export "$key"
done < "$ENV_FILE"

iface="${AMNEZIA_INTERFACE_NAME:-awg0}"
expected_port="${AMNEZIA_PORT:-${AMNEZIA_LISTEN_PORT:-}}"
if [[ -z "${expected_port}" ]]; then
  echo "amnezia.server_config.write.error path=$CONF_PATH iface=$iface reason=amnezia_port_missing" >&2
  exit 1
fi

mkdir -p "$(dirname "$CONF_PATH")"
echo "amnezia.server_config.write.start path=$CONF_PATH iface=$iface expected_port=$expected_port" >&2

private_key=""
if [[ -f "$CONF_PATH" ]]; then
  private_key="$(awk -F'=' '/^[[:space:]]*PrivateKey[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}' "$CONF_PATH")"
fi
if [[ -z "$private_key" ]]; then
  private_key="${AMNEZIA_PRIVATE_KEY:-}"
fi

if [[ -z "$private_key" ]]; then
  echo "amnezia.server_config.write.error path=$CONF_PATH iface=$iface reason=private_key_missing" >&2
  exit 1
fi

tmp="$(mktemp)"
{
  echo "[Interface]"
  echo "PrivateKey = $private_key"
  echo "ListenPort = $expected_port"
} > "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$CONF_PATH"

actual_port="$(awk -F'=' '/^[[:space:]]*ListenPort[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}' "$CONF_PATH")"
if [[ "$actual_port" != "$expected_port" ]]; then
  echo "amnezia.server_config.write.error path=$CONF_PATH iface=$iface expected_port=$expected_port actual_port=${actual_port:-unknown} reason=port_mismatch_after_write" >&2
  exit 1
fi

echo "amnezia.server_config.write.success path=$CONF_PATH iface=$iface listen_port=$actual_port" >&2
