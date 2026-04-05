#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${1:-.env.node.generated}"
CONFIG_PATH="${2:-deploy/node/xray/config.json}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "xray.server_config.write.error env_file=$ENV_FILE reason=env_file_not_found" >&2
  exit 1
fi
if [[ ! -f "$CONFIG_PATH" ]]; then
  echo "xray.server_config.write.error path=$CONFIG_PATH reason=config_not_found" >&2
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

xray_port="${XRAY_REALITY_PORT:-8443}"
xray_sni="${XRAY_REALITY_SERVER_NAME:-www.cloudflare.com}"
xray_short_id="${XRAY_REALITY_SHORT_ID:-0123456789abcdef}"
xray_private_key="${XRAY_REALITY_PRIVATE_KEY:-}"

if [[ -z "$xray_private_key" ]]; then
  echo "xray.server_config.write.error path=$CONFIG_PATH reason=private_key_missing env=XRAY_REALITY_PRIVATE_KEY" >&2
  exit 1
fi

echo "xray.server_config.write.start path=$CONFIG_PATH port=$xray_port server_name=$xray_sni short_id=$xray_short_id" >&2

python3 - "$CONFIG_PATH" "$xray_port" "$xray_sni" "$xray_short_id" "$xray_private_key" <<'PY'
import json
import sys
from pathlib import Path

config_path = Path(sys.argv[1])
port = int(sys.argv[2])
server_name = sys.argv[3]
short_id = sys.argv[4]
private_key = sys.argv[5]

data = json.loads(config_path.read_text(encoding="utf-8"))
inbounds = data.get("inbounds") or []
if not inbounds:
    raise SystemExit("xray config has no inbounds")

target = inbounds[0]
target["port"] = port
stream = target.setdefault("streamSettings", {})
reality = stream.setdefault("realitySettings", {})
reality["serverNames"] = [server_name]
reality["dest"] = f"{server_name}:443"
reality["privateKey"] = private_key
reality["shortIds"] = [short_id]

config_path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

echo "xray.server_config.write.success path=$CONFIG_PATH port=$xray_port server_name=$xray_sni short_id=$xray_short_id" >&2
