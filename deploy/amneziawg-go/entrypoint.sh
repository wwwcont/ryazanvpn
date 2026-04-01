#!/bin/sh
set -eu

IFACE="${AMNEZIA_INTERFACE_NAME:-awg0}"
CONF_DIR="${AMNEZIA_CONFIG_DIR:-/etc/amnezia}"
CONFIG_PATH="${AMNEZIA_CONFIG_PATH:-}"
KEEP_UP="${AMNEZIA_KEEP_INTERFACE_UP:-1}"
SOCK_DIR="/var/run/amneziawg"
SOCK_PATH="${SOCK_DIR}/${IFACE}.sock"

find_config() {
  if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
    echo "$CONFIG_PATH"
    return
  fi

  for candidate in \
    "$CONF_DIR/$IFACE.conf" \
    "$CONF_DIR/amneziawg.conf" \
    "$CONF_DIR/wg0.conf" \
    "$CONF_DIR/server.conf"
  do
    if [ -f "$candidate" ]; then
      echo "$candidate"
      return
    fi
  done
}

create_default_config() {
  default_conf="$CONF_DIR/$IFACE.conf"
  if [ -f "$default_conf" ]; then
    echo "$default_conf"
    return
  fi

  mkdir -p "$CONF_DIR"
  private_key="$(awg genkey)"
  umask 077
  cat > "$default_conf" <<EOF
[Interface]
PrivateKey = $private_key
ListenPort = ${AMNEZIA_LISTEN_PORT:-51820}
EOF
  echo "$default_conf"
}

extract_addresses() {
  conf="$1"
  awk '
    BEGIN { in_interface = 0 }
    /^[[:space:]]*\[Interface\][[:space:]]*$/ { in_interface = 1; next }
    /^[[:space:]]*\[/ { in_interface = 0; next }
    in_interface {
      line = $0
      sub(/[[:space:]]*[#;].*$/, "", line)
      if (line ~ /^[[:space:]]*Address[[:space:]]*=/) {
        split(line, kv, "=")
        value = kv[2]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        n = split(value, arr, /,/)
        for (i = 1; i <= n; i++) {
          addr = arr[i]
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", addr)
          if (addr != "") print addr
        }
      }
    }
  ' "$conf"
}

wait_for_runtime() {
  i=0
  while [ $i -lt 50 ]; do
    if [ -S "$SOCK_PATH" ] || ip link show "$IFACE" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 0.2
  done

  echo "amneziawg-go runtime did not become ready for $IFACE" >&2
  return 1
}

start_runtime() {
  mkdir -p "$SOCK_DIR"
  AMNEZIAWG_PROCESS_FOREGROUND=1 LOG_LEVEL="${LOG_LEVEL:-info}" \
    /usr/local/bin/amneziawg-go -f "$IFACE" &
  RUNTIME_PID=$!
  export RUNTIME_PID
  wait_for_runtime
}

apply_config() {
  conf="$1"
  /usr/local/bin/awg setconf "$IFACE" "$conf"

  extract_addresses "$conf" | while IFS= read -r addr; do
    [ -n "$addr" ] || continue
    ip addr add "$addr" dev "$IFACE" 2>/dev/null || true
  done

  ip link set up dev "$IFACE"
}

shutdown() {
  if [ "${RUNTIME_PID:-}" != "" ]; then
    kill "$RUNTIME_PID" 2>/dev/null || true
    wait "$RUNTIME_PID" 2>/dev/null || true
  fi

  if [ "$KEEP_UP" != "1" ]; then
    ip link del "$IFACE" 2>/dev/null || true
    rm -f "$SOCK_PATH" 2>/dev/null || true
  fi
}

main() {
  conf="$(find_config || true)"
  if [ -z "$conf" ]; then
    echo "config not found in $CONF_DIR; generating minimal bootstrap config" >&2
    conf="$(create_default_config)"
  fi

  start_runtime
  apply_config "$conf"

  trap 'shutdown; exit 0' INT TERM
  wait "$RUNTIME_PID"
}

main "$@"