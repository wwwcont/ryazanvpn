#!/bin/sh
set -eu

IFACE="${AMNEZIA_INTERFACE_NAME:-awg0}"
CONF_DIR="${AMNEZIA_CONFIG_DIR:-/etc/amnezia}"
CONFIG_PATH="${AMNEZIA_CONFIG_PATH:-}"
KEEP_UP="${AMNEZIA_KEEP_INTERFACE_UP:-1}"

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

create_interface() {
  if ip link show "$IFACE" >/dev/null 2>&1; then
    return
  fi

  ip link add "$IFACE" type amneziawg 2>/dev/null \
    || ip link add "$IFACE" type wireguard 2>/dev/null \
    || true

  if ! ip link show "$IFACE" >/dev/null 2>&1; then
    echo "failed to create interface $IFACE (neither amneziawg nor wireguard type available)" >&2
    exit 1
  fi
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
  if [ "$KEEP_UP" = "1" ]; then
    return
  fi
  ip link del "$IFACE" 2>/dev/null || true
}

main() {
  create_interface

  conf="$(find_config || true)"
  if [ -n "$conf" ]; then
    apply_config "$conf"
  else
    echo "kernel-backed awg runtime: config not found in $CONF_DIR" >&2
  fi

  trap 'shutdown; exit 0' INT TERM
  while :; do sleep 3600; done
}

main "$@"
