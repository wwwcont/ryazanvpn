#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env.single.generated}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "sync-runtime-from-configs: env file not found: $ENV_FILE" >&2
  exit 1
fi

python3 - "$ENV_FILE" <<'PY'
import json
import re
import sys
from pathlib import Path

env_path = Path(sys.argv[1])
text = env_path.read_text(encoding="utf-8")
lines = text.splitlines()
pattern = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")

def parse_env(lines):
    out = {}
    for line in lines:
        m = pattern.match(line)
        if m:
            out[m.group(1)] = m.group(2)
    return out

def read_text(path_str: str) -> str:
    path = Path(path_str.strip())
    return path.read_text(encoding="utf-8")

def read_trimmed(path_str: str) -> str:
    return read_text(path_str).strip()

def endpoint_with_port(host_or_endpoint: str, port: str) -> str:
    value = host_or_endpoint.strip()
    if not value:
        return ""
    if ":" in value:
        value = value.rsplit(":", 1)[0]
    return f"{value}:{port}"

env = parse_env(lines)
updates = {}

amnezia_cfg_path = (env.get("AMNEZIA_CONFIG_PATH") or "").strip()
if not amnezia_cfg_path:
    raise SystemExit("sync-runtime-from-configs: AMNEZIA_CONFIG_PATH is required")

am_text = read_text(amnezia_cfg_path)
listen_port = ""
subnet = ""
for raw in am_text.splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    if "=" not in line:
        continue
    k, v = [x.strip() for x in line.split("=", 1)]
    kl = k.lower()
    if kl == "listenport":
        listen_port = v
    elif kl == "address" and not subnet:
        subnet = v.split(",")[0].strip()

if not listen_port:
    raise SystemExit("sync-runtime-from-configs: ListenPort not found in AMNEZIA_CONFIG_PATH")

updates["AMNEZIA_PORT"] = listen_port
if subnet:
    updates["VPN_SUBNET_CIDR"] = subnet

vpn_host = (env.get("VPN_PUBLIC_HOST") or env.get("NODE_PUBLIC_IP") or env.get("VPN_SERVER_PUBLIC_ENDPOINT") or "").strip()
endpoint = endpoint_with_port(vpn_host, listen_port)
if endpoint:
    updates["VPN_SERVER_PUBLIC_ENDPOINT"] = endpoint

amnezia_pub_path = (env.get("AMNEZIA_PUBLIC_KEY_SOURCE_PATH") or env.get("VPN_SERVER_PUBLIC_KEY_FILE") or "").strip()
if amnezia_pub_path:
    updates["VPN_SERVER_PUBLIC_KEY"] = read_trimmed(amnezia_pub_path)

xray_cfg_path = (env.get("XRAY_SOURCE_CONFIG_PATH") or "").strip()
if not xray_cfg_path:
    raise SystemExit("sync-runtime-from-configs: XRAY_SOURCE_CONFIG_PATH is required")

xray = json.loads(read_text(xray_cfg_path))
inbounds = xray.get("inbounds") or []
if not inbounds:
    raise SystemExit("sync-runtime-from-configs: XRAY_SOURCE_CONFIG_PATH has no inbounds")

inbound = inbounds[0]
port = inbound.get("port")
if port is not None:
    updates["XRAY_REALITY_PORT"] = str(port)

reality = ((inbound.get("streamSettings") or {}).get("realitySettings") or {})
server_names = reality.get("serverNames") or []
short_ids = reality.get("shortIds") or []
if server_names:
    updates["XRAY_REALITY_SERVER_NAME"] = str(server_names[0])
if short_ids:
    updates["XRAY_REALITY_SHORT_ID"] = str(short_ids[0])

if not (env.get("XRAY_PUBLIC_HOST") or "").strip() and server_names:
    updates["XRAY_PUBLIC_HOST"] = str(server_names[0])

xray_pub_path = (env.get("XRAY_REALITY_PUBLIC_KEY_SOURCE_PATH") or env.get("XRAY_REALITY_PUBLIC_KEY_FILE") or "").strip()
if xray_pub_path:
    updates["XRAY_REALITY_PUBLIC_KEY"] = read_trimmed(xray_pub_path)

if not updates.get("VPN_SERVER_PUBLIC_KEY") and not (env.get("VPN_SERVER_PUBLIC_KEY") or "").strip():
    raise SystemExit("sync-runtime-from-configs: set AMNEZIA_PUBLIC_KEY_SOURCE_PATH or VPN_SERVER_PUBLIC_KEY")
if not updates.get("XRAY_REALITY_PUBLIC_KEY") and not (env.get("XRAY_REALITY_PUBLIC_KEY") or "").strip():
    raise SystemExit("sync-runtime-from-configs: set XRAY_REALITY_PUBLIC_KEY_SOURCE_PATH or XRAY_REALITY_PUBLIC_KEY")

seen = set()
out_lines = []
for line in lines:
    m = pattern.match(line)
    if not m:
        out_lines.append(line)
        continue
    key = m.group(1)
    if key in updates:
        out_lines.append(f"{key}={updates[key]}")
        seen.add(key)
    else:
        out_lines.append(line)

for key, value in updates.items():
    if key not in seen:
        out_lines.append(f"{key}={value}")

env_path.write_text("\n".join(out_lines) + "\n", encoding="utf-8")
print("sync-runtime-from-configs: updated keys:", ", ".join(sorted(updates.keys())))
PY
