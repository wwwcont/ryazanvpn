#!/bin/sh
set -eu

WG_BIN="$(command -v wg)"

if [ "${AWG_USE_WG_FALLBACK:-1}" = "1" ] && [ "${1:-}" = "setconf" ] && [ $# -ge 3 ]; then
  iface="$2"
  src_conf="$3"
  tmp_conf="$(mktemp)"
  trap 'rm -f "$tmp_conf"' EXIT INT TERM

  awk '
    /^[[:space:]]*\[/ { print; next }
    /^[[:space:]]*#/ { print; next }
    /^[[:space:]]*;/ { print; next }
    /^[[:space:]]*$/ { print; next }
    {
      line=$0
      key=line
      sub(/[[:space:]]*=.*/, "", key)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
      if (key == "Address" || key == "DNS" || key == "Jc" || key == "Jmin" || key == "Jmax" ||
          key == "S1" || key == "S2" || key == "S3" || key == "S4" ||
          key == "H1" || key == "H2" || key == "H3" || key == "H4" ||
          key == "I1" || key == "I2" || key == "I3" || key == "I4" || key == "I5") {
        next
      }
      print line
    }
  ' "$src_conf" > "$tmp_conf"

  exec "$WG_BIN" setconf "$iface" "$tmp_conf"
fi

exec "$WG_BIN" "$@"
