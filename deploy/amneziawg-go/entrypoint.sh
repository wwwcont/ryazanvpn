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
NAT_ENABLED="${AMNEZIA_NAT_ENABLED:-1}"
EGRESS_IFACE="${AMNEZIA_EGRESS_IFACE:-}"
PUBLIC_KEY_PATH="${AMNEZIA_PUBLIC_KEY_PATH:-/etc/amnezia/server.publickey}"

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

ensure_listen_port() {
  conf="$1"
  [ -n "$EXPECTED_PORT" ] || return 0
  tmp="${conf}.tmp.$$"
  awk -v expected="$EXPECTED_PORT" '
    BEGIN { in_interface = 0; inserted = 0 }
    /^[[:space:]]*\[Interface\][[:space:]]*$/ {
      if (in_interface && inserted == 0) {
        print "ListenPort = " expected
        inserted = 1
      }
      in_interface = 1
      print
      next
    }
    /^[[:space:]]*\[/ {
      if (in_interface && inserted == 0) {
        print "ListenPort = " expected
        inserted = 1
      }
      in_interface = 0
      print
      next
    }
    {
      if (in_interface && $0 ~ /^[[:space:]]*ListenPort[[:space:]]*=/) {
        print "ListenPort = " expected
        inserted = 1
        next
      }
      print
    }
    END {
      if (in_interface && inserted == 0) {
        print "ListenPort = " expected
      }
    }
  ' "$conf" > "$tmp"
  mv "$tmp" "$conf"
}

extract_interface_private_key() {
  conf="$1"
  awk '
    BEGIN { in_interface = 0 }
    /^[[:space:]]*\[Interface\][[:space:]]*$/ { in_interface = 1; next }
    /^[[:space:]]*\[/ { in_interface = 0; next }
    in_interface {
      line = $0
      sub(/[[:space:]]*[#;].*$/, "", line)
      if (line ~ /^[[:space:]]*PrivateKey[[:space:]]*=/) {
        split(line, kv, "=")
        value = kv[2]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        if (value != "") {
          print value
          exit 0
        }
      }
    }
  ' "$conf"
}

write_server_public_key() {
  conf="$1"
  log_mode="${2:-ready}"
  private_key="$(extract_interface_private_key "$conf")"
  if [ -z "$private_key" ]; then
    echo "amnezia.server_public_key.warning reason=private_key_not_found config=$conf" >&2
    return 1
  fi

  if ! public_key="$(printf '%s\n' "$private_key" | awg pubkey 2>/dev/null)"; then
    echo "amnezia.server_public_key.warning reason=pubkey_calculation_failed config=$conf" >&2
    return 1
  fi

  public_key="$(printf '%s' "$public_key" | tr -d '\r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [ -z "$public_key" ]; then
    echo "amnezia.server_public_key.warning reason=empty_pubkey config=$conf" >&2
    return 1
  fi

  public_key_dir="$(dirname "$PUBLIC_KEY_PATH")"
  if ! mkdir -p "$public_key_dir" 2>/dev/null; then
    echo "amnezia.server_public_key.warning reason=directory_create_failed path=$PUBLIC_KEY_PATH" >&2
    return 1
  fi
  umask 077
  if ! printf '%s\n' "$public_key" > "$PUBLIC_KEY_PATH"; then
    echo "amnezia.server_public_key.warning reason=file_write_failed path=$PUBLIC_KEY_PATH" >&2
    return 1
  fi
  case "$log_mode" in
    reused)
      echo "amnezia.server_public_key.reused path=$PUBLIC_KEY_PATH" >&2
      ;;
    *)
      echo "amnezia.server_public_key.ready path=$PUBLIC_KEY_PATH" >&2
      ;;
  esac
}

read_public_key_file() {
  if [ ! -f "$PUBLIC_KEY_PATH" ]; then
    return 1
  fi
  key="$(tr -d '\r\n' < "$PUBLIC_KEY_PATH" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [ -z "$key" ]; then
    return 1
  fi
  echo "$key"
}

ensure_server_public_key() {
  conf="$1"
  mode="$2"

  if read_public_key_file >/dev/null 2>&1; then
    echo "amnezia.server_public_key.reused path=$PUBLIC_KEY_PATH" >&2
    return 0
  fi

  if [ "$mode" = "generated" ]; then
    if ! write_server_public_key "$conf" ready; then
      echo "amnezia.server_public_key.warning reason=initial_write_failed path=$PUBLIC_KEY_PATH config=$conf" >&2
    fi
    return 0
  fi

  if ! write_server_public_key "$conf" reused; then
    echo "amnezia.server_public_key.warning reason=recover_failed path=$PUBLIC_KEY_PATH config=$conf" >&2
  fi
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

detect_egress_iface() {
  if [ -n "$EGRESS_IFACE" ]; then
    echo "$EGRESS_IFACE"
    return
  fi
  iface="$(ip route show default 2>/dev/null | awk '/default/ {print $5; exit}')"
  if [ -n "$iface" ]; then
    echo "$iface"
    return
  fi
  echo "eth0"
}

ensure_rule() {
  table="$1"
  shift
  if [ "$table" = "nat" ]; then
    if iptables -t nat -C "$@" 2>/dev/null; then
      return 0
    fi
    iptables -t nat -A "$@"
    return 0
  fi

  if iptables -C "$@" 2>/dev/null; then
    return 0
  fi
  iptables -A "$@"
}

setup_forwarding() {
  if [ "$NAT_ENABLED" != "1" ]; then
    echo "amnezia.forwarding.disabled nat_enabled=$NAT_ENABLED" >&2
    return 0
  fi

  egress="$(detect_egress_iface)"
  sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true
  ensure_rule nat POSTROUTING -s "$DEFAULT_SUBNET" -o "$egress" -j MASQUERADE
  ensure_rule filter FORWARD -i "$IFACE" -o "$egress" -j ACCEPT
  ensure_rule filter FORWARD -i "$egress" -o "$IFACE" -m state --state RELATED,ESTABLISHED -j ACCEPT
  echo "amnezia.forwarding.ready iface=$IFACE subnet=$DEFAULT_SUBNET egress=$egress" >&2
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
  config_mode="existing"
  conf="$(find_config || true)"
  if [ -z "$conf" ]; then
    echo "amnezia.server_config.not_found iface=$IFACE conf_dir=$CONF_DIR action=generating_from_env" >&2
    conf="$(create_config_from_env)"
    config_mode="generated"
  fi
  ensure_listen_port "$conf"
  echo "amnezia.server_config.selected path=$conf" >&2
  ensure_server_public_key "$conf" "$config_mode"

  start_runtime
  apply_config "$conf"
  setup_forwarding
  actual_port="$(awg show "$IFACE" listen-port 2>/dev/null || true)"
  echo "amnezia.server_listen_port.expected value=${EXPECTED_PORT:-unknown}" >&2
  echo "amnezia.server_listen_port.actual value=${actual_port:-unknown}" >&2
  if [ -n "$EXPECTED_PORT" ] && [ "$actual_port" != "$EXPECTED_PORT" ]; then
    echo "amnezia.server_listen_port.mismatch expected=$EXPECTED_PORT actual=${actual_port:-unknown}" >&2
    exit 1
  fi

  trap 'shutdown; exit 0' INT TERM
  wait "$RUNTIME_PID"
}

main "$@"
