#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env.generated}"
AMNEZIA_META_PATH="${AMNEZIA_RUNTIME_METADATA_PATH:-deploy/node/amnezia/runtime-metadata.json}"
XRAY_META_PATH="${XRAY_RUNTIME_METADATA_PATH:-deploy/node/xray/runtime-metadata.json}"
AMNEZIA_PUB_PATH="${AMNEZIA_PUBLIC_KEY_PATH:-deploy/node/amnezia/server.publickey}"
XRAY_PUB_PATH="${XRAY_REALITY_PUBLIC_KEY_FILE:-deploy/node/xray/reality.publickey}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE" >&2
  exit 1
fi

python3 - "$ENV_FILE" "$AMNEZIA_META_PATH" "$XRAY_META_PATH" "$AMNEZIA_PUB_PATH" "$XRAY_PUB_PATH" <<'PY'
import json
import re
import sys
from pathlib import Path

env_path = Path(sys.argv[1])
amnezia_meta_path = Path(sys.argv[2])
xray_meta_path = Path(sys.argv[3])
amnezia_pub_path = Path(sys.argv[4])
xray_pub_path = Path(sys.argv[5])

def read_json(path: Path):
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}

def read_trimmed(path: Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8").strip()

def parse_env(lines):
    pattern = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")
    values = {}
    for line in lines:
        m = pattern.match(line)
        if m:
            values[m.group(1)] = m.group(2)
    return values

def endpoint_with_port(host_or_endpoint: str, port: str) -> str:
    value = host_or_endpoint.strip()
    if not value:
        return ""
    if ":" in value:
        value = value.rsplit(":", 1)[0]
    return f"{value}:{port}"

text = env_path.read_text(encoding="utf-8")
lines = text.splitlines()
existing = parse_env(lines)
updates = {}

am = read_json(amnezia_meta_path)
am_port = ""
if am:
    if am.get("listen_port"):
        am_port = str(am["listen_port"])
        updates["AMNEZIA_PORT"] = am_port
    if am.get("subnet"):
        updates["AMNEZIA_SUBNET"] = str(am["subnet"])
        updates["VPN_SUBNET_CIDR"] = str(am["subnet"])

    for src, key in [
        ("jc", "VPN_AWG_JC"), ("jmin", "VPN_AWG_JMIN"), ("jmax", "VPN_AWG_JMAX"),
        ("s1", "VPN_AWG_S1"), ("s2", "VPN_AWG_S2"), ("s3", "VPN_AWG_S3"), ("s4", "VPN_AWG_S4"),
        ("h1", "VPN_AWG_H1"), ("h2", "VPN_AWG_H2"), ("h3", "VPN_AWG_H3"), ("h4", "VPN_AWG_H4"),
        ("i1", "VPN_AWG_I1"), ("i2", "VPN_AWG_I2"), ("i3", "VPN_AWG_I3"), ("i4", "VPN_AWG_I4"), ("i5", "VPN_AWG_I5"),
    ]:
        v = am.get(src)
        if v is not None and str(v).strip() != "":
            updates[key] = str(v)

am_pub = read_trimmed(amnezia_pub_path) or str(am.get("server_public_key", "")).strip()
if am_pub:
    updates["VPN_SERVER_PUBLIC_KEY"] = am_pub

if am_port:
    preferred_host = (existing.get("VPN_PUBLIC_HOST") or existing.get("NODE_PUBLIC_IP") or "").strip()
    current_endpoint = existing.get("VPN_SERVER_PUBLIC_ENDPOINT", "")
    candidate = endpoint_with_port(preferred_host, am_port) or endpoint_with_port(current_endpoint, am_port)
    if candidate:
        updates["VPN_SERVER_PUBLIC_ENDPOINT"] = candidate

xr = read_json(xray_meta_path)
if xr:
    if xr.get("listen_port"):
        updates["XRAY_REALITY_PORT"] = str(xr["listen_port"])
    if xr.get("server_name"):
        updates["XRAY_REALITY_SERVER_NAME"] = str(xr["server_name"])
    if xr.get("short_id"):
        updates["XRAY_REALITY_SHORT_ID"] = str(xr["short_id"])
    if xr.get("public_host"):
        updates["XRAY_PUBLIC_HOST"] = str(xr["public_host"])

xr_pub = read_trimmed(xray_pub_path) or str(xr.get("reality_public_key", "")).strip()
if xr_pub:
    updates["XRAY_REALITY_PUBLIC_KEY"] = xr_pub

if not updates:
    print("runtime-sync-env: no runtime values found; env unchanged")
    raise SystemExit(0)

pattern = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")
seen = set()
out = []
for line in lines:
    m = pattern.match(line)
    if not m:
        out.append(line)
        continue
    k = m.group(1)
    if k in updates:
        out.append(f"{k}={updates[k]}")
        seen.add(k)
    else:
        out.append(line)

for k, v in updates.items():
    if k not in seen:
        out.append(f"{k}={v}")

env_path.write_text("\n".join(out) + "\n", encoding="utf-8")
print("runtime-sync-env: updated keys:", ", ".join(sorted(updates.keys())))
PY
