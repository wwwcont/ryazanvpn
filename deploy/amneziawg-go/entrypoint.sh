#!/bin/sh
set -eu

IFACE="${AMNEZIA_INTERFACE_NAME:-awg0}"
CONF_DIR="${AMNEZIA_CONFIG_DIR:-/etc/amnezia}"
CONFIG_PATH="${AMNEZIA_CONFIG_PATH:-}"
KEEP_UP="${AMNEZIA_KEEP_INTERFACE_UP:-1}"
SOCK_DIR="/var/run/amneziawg"
SOCK_PATH="${SOCK_DIR}/${IFACE}.sock"
EXPECTED_PORT="${AMNEZIA_PORT:-${AMNEZIA_LISTEN_PORT:-}}"
DEFAULT_SUBNET="${AMNEZIA_SUBNET:-10.8.1.0/24}"

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

derive_interface_address() {
  if [ -n "${AMNEZIA_ADDRESS:-}" ]; then
    echo "$AMNEZIA_ADDRESS"
    return
  fi
  subnet="$DEFAULT_SUBNET"
  base="${subnet%%/*}"
  mask="${subnet##*/}"
  if echo "$base" | awk -F. 'NF==4 {exit 0} {exit 1}'; then
    echo "$base" | awk -F. -v m="$mask" '{printf "%s.%s.%s.1/%s\n",$1,$2,$3,m}'
    return
  fi
  echo ""
}

create_config_from_env() {
  if [ -z "$EXPECTED_PORT" ]; then
    echo "AMNEZIA_PORT (or AMNEZIA_LISTEN_PORT) is required to generate server config" >&2
    return 1
  fi
  mkdir -p "$CONF_DIR"
  conf="$CONF_DIR/$IFACE.conf"
  key="${AMNEZIA_PRIVATE_KEY:-}"
  if [ -z "$key" ]; then
    key="$(awg genkey)"
  fi
  iface_addr="$(derive_interface_address)"
  umask 077
  {
    echo "[Interface]"
    echo "PrivateKey = $key"
    echo "ListenPort = $EXPECTED_PORT"
    if [ -n "$iface_addr" ]; then
      echo "Address = $iface_addr"
    fi
  } > "$conf"
  echo "$conf"
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
    echo "amnezia.server_config.not_found iface=$IFACE conf_dir=$CONF_DIR action=generating_from_env" >&2
    conf="$(create_config_from_env)"
  fi
  echo "amnezia.server_config.loaded path=$conf" >&2

  start_runtime
  apply_config "$conf"
  actual_port="$(awg show "$IFACE" listen-port 2>/dev/null || true)"
  echo "amnezia.server_listen_port.expected value=${EXPECTED_PORT:-unknown}" >&2
  echo "amnezia.server_listen_port.actual value=${actual_port:-unknown}" >&2

  trap 'shutdown; exit 0' INT TERM
  wait "$RUNTIME_PID"
}

main "$@"
