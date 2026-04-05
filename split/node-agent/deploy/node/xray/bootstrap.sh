#!/bin/sh
set -eu

CONFIG_PATH="${XRAY_CONFIG_PATH:-/etc/xray/config.json}"
METADATA_PATH="${XRAY_RUNTIME_METADATA_PATH:-/etc/xray/runtime-metadata.json}"
LISTEN_PORT="${XRAY_REALITY_PORT:-8443}"
SERVER_NAME="${XRAY_REALITY_SERVER_NAME:-www.cloudflare.com}"
SHORT_ID="${XRAY_REALITY_SHORT_ID:-0123456789abcdef}"
PUBLIC_HOST="${XRAY_PUBLIC_HOST:-}"
ENV_PRIVATE_KEY="${XRAY_REALITY_PRIVATE_KEY:-}"
ENV_PUBLIC_KEY="${XRAY_REALITY_PUBLIC_KEY:-}"

trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

read_json_field() {
  field="$1"
  compact="$(tr -d '\n\r' < "$CONFIG_PATH" 2>/dev/null || true)"
  case "$field" in
    privateKey)
      printf '%s' "$compact" | sed -n 's/.*"privateKey"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
      ;;
    shortId)
      printf '%s' "$compact" | sed -n 's/.*"shortIds"[[:space:]]*:[[:space:]]*\[[[:space:]]*"\([^"]*\)".*/\1/p'
      ;;
    serverName)
      printf '%s' "$compact" | sed -n 's/.*"serverNames"[[:space:]]*:[[:space:]]*\[[[:space:]]*"\([^"]*\)".*/\1/p'
      ;;
    port)
      printf '%s' "$compact" | sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p'
      ;;
    dest)
      printf '%s' "$compact" | sed -n 's/.*"dest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
      ;;
    *)
      printf ''
      ;;
  esac
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
  "config_path": "$(json_escape "$CONFIG_PATH")",
  "runtime_source": "xray",
  "private_key_present": $( [ -n "$private_key" ] && printf 'true' || printf 'false')
}
EOF
}

if [ -z "$ENV_PRIVATE_KEY" ]; then
  echo "xray.bootstrap.error reason=private_key_missing env=XRAY_REALITY_PRIVATE_KEY" >&2
  exit 1
fi
if [ -z "$ENV_PUBLIC_KEY" ]; then
  echo "xray.bootstrap.error reason=public_key_missing env=XRAY_REALITY_PUBLIC_KEY" >&2
  exit 1
fi
echo "xray.reality.private_key.source=env" >&2
echo "xray.reality.public_key.source=env" >&2

if [ ! -f "$CONFIG_PATH" ]; then
  mkdir -p "$(dirname "$CONFIG_PATH")"
  umask 077
  cat > "$CONFIG_PATH" <<EOF
{
  "api": {
    "tag": "api",
    "services": [
      "HandlerService",
      "StatsService"
    ]
  },
  "policy": {
    "levels": {
      "0": {
        "statsUserUplink": true,
        "statsUserDownlink": true
      }
    },
    "system": {
      "statsInboundUplink": true,
      "statsInboundDownlink": true,
      "statsOutboundUplink": true,
      "statsOutboundDownlink": true
    }
  },
  "stats": {},
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "tag": "api",
      "listen": "127.0.0.1",
      "port": 10085,
      "protocol": "dokodemo-door",
      "settings": {
        "address": "127.0.0.1"
      }
    },
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
          "privateKey": "$(json_escape "$ENV_PRIVATE_KEY")",
          "shortIds": [
            "$(json_escape "$SHORT_ID")"
          ]
        }
      }
    }
  ],
  "outbounds": [
    {
      "tag": "api",
      "protocol": "freedom"
    },
    {
      "protocol": "freedom",
      "tag": "direct"
    }
  ],
  "routing": {
    "rules": [
      {
        "type": "field",
        "inboundTag": [
          "api"
        ],
        "outboundTag": "api"
      }
    ]
  ]
}
EOF
  write_metadata "$ENV_PUBLIC_KEY" "$SHORT_ID" "$SERVER_NAME" "$LISTEN_PORT" "$ENV_PRIVATE_KEY" || echo "xray.bootstrap.warning reason=metadata_write_failed path=$METADATA_PATH" >&2
  echo "xray.bootstrap.generated config=$CONFIG_PATH" >&2
else
  private_key="$(trim "$(read_json_field privateKey)")"
  short_id="$(trim "$(read_json_field shortId)")"
  server_name="$(trim "$(read_json_field serverName)")"
  listen_port="$(trim "$(read_json_field port)")"

  [ -n "$short_id" ] || short_id="$SHORT_ID"
  [ -n "$server_name" ] || server_name="$SERVER_NAME"
  [ -n "$listen_port" ] || listen_port="$LISTEN_PORT"
  if [ -z "$private_key" ]; then
    echo "xray.bootstrap.error reason=runtime_private_key_missing config=$CONFIG_PATH" >&2
    exit 1
  fi
  if [ "$private_key" != "$ENV_PRIVATE_KEY" ]; then
    echo "xray.bootstrap.error reason=private_key_mismatch config=$CONFIG_PATH env=XRAY_REALITY_PRIVATE_KEY" >&2
    exit 1
  fi
  write_metadata "$ENV_PUBLIC_KEY" "$short_id" "$server_name" "$listen_port" "$private_key" || echo "xray.bootstrap.warning reason=metadata_write_failed path=$METADATA_PATH" >&2
  echo "xray.bootstrap.reused config=$CONFIG_PATH" >&2
fi
