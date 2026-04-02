#!/bin/sh
set -eu

CONFIG_PATH="${XRAY_CONFIG_PATH:-/etc/xray/config.json}"
METADATA_PATH="${XRAY_RUNTIME_METADATA_PATH:-/etc/xray/runtime-metadata.json}"
PUBLIC_KEY_PATH="${XRAY_REALITY_PUBLIC_KEY_FILE:-/etc/xray/reality.publickey}"
LISTEN_PORT="${XRAY_REALITY_PORT:-8443}"
SERVER_NAME="${XRAY_REALITY_SERVER_NAME:-www.cloudflare.com}"
SHORT_ID="${XRAY_REALITY_SHORT_ID:-0123456789abcdef}"
PUBLIC_HOST="${XRAY_PUBLIC_HOST:-}"

trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

xray_genkeypair() {
  out="$(xray x25519 2>/dev/null || true)"
  private="$(printf '%s\n' "$out" | awk -F': *' '/Private key/ {print $2; exit}')"
  public="$(printf '%s\n' "$out" | awk -F': *' '/Public key/ {print $2; exit}')"
  private="$(trim "$private")"
  public="$(trim "$public")"
  if [ -n "$private" ] && [ -n "$public" ]; then
    printf '%s\n%s\n' "$private" "$public"
    return 0
  fi
  return 1
}

derive_public_from_private() {
  private="$1"
  [ -n "$private" ] || return 1
  out="$(printf '%s\n' "$private" | xray x25519 -i 2>/dev/null || true)"
  public="$(printf '%s\n' "$out" | awk -F': *' '/Public key/ {print $2; exit}')"
  public="$(trim "$public")"
  [ -n "$public" ] || return 1
  printf '%s\n' "$public"
}

read_json_field() {
  field="$1"
  python3 - "$CONFIG_PATH" "$field" <<'PY'
import json
import sys
from pathlib import Path

cfg_path = Path(sys.argv[1])
field = sys.argv[2]
try:
    data = json.loads(cfg_path.read_text(encoding="utf-8"))
    inbound = (data.get("inbounds") or [])[0]
    rs = (((inbound.get("streamSettings") or {}).get("realitySettings") or {}))
except Exception:
    print("")
    raise SystemExit(0)

if field == "privateKey":
    print(str(rs.get("privateKey", "")))
elif field == "shortId":
    ids = rs.get("shortIds") or []
    print(str(ids[0] if ids else ""))
elif field == "serverName":
    names = rs.get("serverNames") or []
    print(str(names[0] if names else ""))
elif field == "port":
    print(str(inbound.get("port", "")))
elif field == "dest":
    print(str(rs.get("dest", "")))
PY
}

write_metadata() {
  public_key="$1"
  short_id="$2"
  server_name="$3"
  listen_port="$4"
  private_key="$5"
  mkdir -p "$(dirname "$METADATA_PATH")"
  umask 077
  cat > "$METADATA_PATH" <<EOF
{
  "listen_port": $listen_port,
  "server_name": "$(json_escape "$server_name")",
  "short_id": "$(json_escape "$short_id")",
  "public_host": "$(json_escape "$PUBLIC_HOST")",
  "reality_public_key": "$(json_escape "$public_key")",
  "reality_public_key_file": "$(json_escape "$PUBLIC_KEY_PATH")",
  "config_path": "$(json_escape "$CONFIG_PATH")",
  "runtime_source": "xray",
  "private_key_present": $( [ -n "$private_key" ] && printf 'true' || printf 'false')
}
EOF
}

ensure_public_key_file() {
  public_key="$1"
  [ -n "$public_key" ] || return 1
  mkdir -p "$(dirname "$PUBLIC_KEY_PATH")"
  umask 077
  printf '%s\n' "$public_key" > "$PUBLIC_KEY_PATH"
}

if [ ! -f "$CONFIG_PATH" ]; then
  keypair="$(xray_genkeypair || true)"
  private_key="$(printf '%s\n' "$keypair" | sed -n '1p')"
  public_key="$(printf '%s\n' "$keypair" | sed -n '2p')"
  if [ -z "$private_key" ] || [ -z "$public_key" ]; then
    echo "xray.bootstrap.error reason=keypair_generation_failed" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$CONFIG_PATH")"
  umask 077
  cat > "$CONFIG_PATH" <<EOF
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "tag": "vless-reality",
      "listen": "0.0.0.0",
      "port": $LISTEN_PORT,
      "protocol": "vless",
      "settings": {
        "clients": [],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "show": false,
          "dest": "$(json_escape "$SERVER_NAME"):443",
          "xver": 0,
          "serverNames": [
            "$(json_escape "$SERVER_NAME")"
          ],
          "privateKey": "$(json_escape "$private_key")",
          "shortIds": [
            "$(json_escape "$SHORT_ID")"
          ]
        }
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "freedom",
      "tag": "direct"
    }
  ]
}
EOF
  ensure_public_key_file "$public_key" || echo "xray.bootstrap.warning reason=public_key_file_write_failed path=$PUBLIC_KEY_PATH" >&2
  write_metadata "$public_key" "$SHORT_ID" "$SERVER_NAME" "$LISTEN_PORT" "$private_key" || echo "xray.bootstrap.warning reason=metadata_write_failed path=$METADATA_PATH" >&2
  echo "xray.bootstrap.generated config=$CONFIG_PATH" >&2
else
  private_key="$(trim "$(read_json_field privateKey)")"
  short_id="$(trim "$(read_json_field shortId)")"
  server_name="$(trim "$(read_json_field serverName)")"
  listen_port="$(trim "$(read_json_field port)")"

  [ -n "$short_id" ] || short_id="$SHORT_ID"
  [ -n "$server_name" ] || server_name="$SERVER_NAME"
  [ -n "$listen_port" ] || listen_port="$LISTEN_PORT"

  public_key=""
  if [ -f "$PUBLIC_KEY_PATH" ]; then
    public_key="$(trim "$(cat "$PUBLIC_KEY_PATH" 2>/dev/null || true)")"
  fi
  if [ -z "$public_key" ]; then
    public_key="$(derive_public_from_private "$private_key" || true)"
    if [ -n "$public_key" ]; then
      ensure_public_key_file "$public_key" || echo "xray.bootstrap.warning reason=public_key_file_write_failed path=$PUBLIC_KEY_PATH" >&2
    fi
  fi

  if [ -n "$public_key" ]; then
    write_metadata "$public_key" "$short_id" "$server_name" "$listen_port" "$private_key" || echo "xray.bootstrap.warning reason=metadata_write_failed path=$METADATA_PATH" >&2
  else
    echo "xray.bootstrap.warning reason=public_key_unavailable config=$CONFIG_PATH" >&2
    if [ ! -f "$METADATA_PATH" ]; then
      write_metadata "" "$short_id" "$server_name" "$listen_port" "$private_key" || true
    fi
  fi
  echo "xray.bootstrap.reused config=$CONFIG_PATH" >&2
fi
