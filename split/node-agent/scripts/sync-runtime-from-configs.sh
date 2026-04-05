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
import subprocess
import sys
from pathlib import Path

env_path = Path(sys.argv[1])
lines = env_path.read_text(encoding="utf-8").splitlines()
pattern = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")

def parse_env(src_lines):
    values = {}
    for line in src_lines:
        m = pattern.match(line)
        if m:
            values[m.group(1)] = m.group(2)
    return values

def run(cmd):
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit(f"sync-runtime-from-configs: command failed: {' '.join(cmd)} :: {p.stderr.strip() or p.stdout.strip()}")
    return p.stdout.strip()

def read_text_from_host_or_container(path: str, *, docker_bin: str, container: str, description: str) -> str:
    src = (path or "").strip()
    if not src:
        raise SystemExit(f"sync-runtime-from-configs: {description} path is required")

    p = Path(src)
    if p.exists():
        return p.read_text(encoding="utf-8")

    if not container:
        raise SystemExit(
            f"sync-runtime-from-configs: {description} not found on host ({src}) and XRAY_CONTAINER_NAME is not set"
        )
    return run([docker_bin, "exec", container, "cat", src])

def endpoint_with_port(host_or_endpoint: str, port: str) -> str:
    value = (host_or_endpoint or "").strip()
    if not value:
        return ""
    if ":" in value:
        value = value.rsplit(":", 1)[0]
    return f"{value}:{port}"

env = parse_env(lines)
updates = {}

# --- Amnezia: source of truth is runtime state ---
docker_bin = (env.get("DOCKER_BINARY_PATH") or "docker").strip() or "docker"
container = (env.get("AMNEZIA_CONTAINER_NAME") or "").strip()
iface = (env.get("AMNEZIA_INTERFACE_NAME") or "").strip()
if not container:
    raise SystemExit("sync-runtime-from-configs: AMNEZIA_CONTAINER_NAME is required")
if not iface:
    raise SystemExit("sync-runtime-from-configs: AMNEZIA_INTERFACE_NAME is required")

amnezia_public_key = run([docker_bin, "exec", container, "awg", "show", iface, "public-key"])
amnezia_port = run([docker_bin, "exec", container, "awg", "show", iface, "listen-port"])
if not amnezia_public_key:
    raise SystemExit("sync-runtime-from-configs: empty Amnezia public key from runtime")
if not amnezia_port:
    raise SystemExit("sync-runtime-from-configs: empty Amnezia listen port from runtime")

updates["VPN_SERVER_PUBLIC_KEY"] = amnezia_public_key
updates["AMNEZIA_PORT"] = amnezia_port

try:
    ip_out = run([docker_bin, "exec", container, "ip", "-o", "-f", "inet", "addr", "show", "dev", iface])
    m = re.search(r"\binet\s+([^\s]+)", ip_out)
    if m:
        updates["VPN_SUBNET_CIDR"] = m.group(1).strip()
except SystemExit:
    # Keep existing VPN_SUBNET_CIDR if ip tool/addr is unavailable in runtime image.
    pass

vpn_host = (env.get("VPN_PUBLIC_HOST") or env.get("NODE_PUBLIC_IP") or env.get("VPN_SERVER_PUBLIC_ENDPOINT") or "").strip()
endpoint = endpoint_with_port(vpn_host, amnezia_port)
if endpoint:
    updates["VPN_SERVER_PUBLIC_ENDPOINT"] = endpoint

# --- Xray: source of truth is configured JSON + env keys ---
xray_cfg_path = (env.get("XRAY_SOURCE_CONFIG_PATH") or "").strip()
if not xray_cfg_path:
    raise SystemExit("sync-runtime-from-configs: XRAY_SOURCE_CONFIG_PATH is required")
xray_container = (env.get("XRAY_CONTAINER_NAME") or "").strip()
xray_private_key_env = (env.get("XRAY_REALITY_PRIVATE_KEY") or "").strip()
xray_public_key_env = (env.get("XRAY_REALITY_PUBLIC_KEY") or "").strip()
if not xray_private_key_env:
    raise SystemExit("sync-runtime-from-configs: XRAY_REALITY_PRIVATE_KEY is required")
if not xray_public_key_env:
    raise SystemExit("sync-runtime-from-configs: XRAY_REALITY_PUBLIC_KEY is required")
print("xray.reality.private_key.source=env")
print("xray.reality.public_key.source=env")

xray_cfg_text = read_text_from_host_or_container(
    xray_cfg_path,
    docker_bin=docker_bin,
    container=xray_container,
    description="XRAY_SOURCE_CONFIG_PATH",
)
xray_data = json.loads(xray_cfg_text)
inbounds = xray_data.get("inbounds") or []
if not inbounds:
    raise SystemExit("sync-runtime-from-configs: XRAY_SOURCE_CONFIG_PATH has no inbounds")

inbound = inbounds[0]
port = inbound.get("port")
if port is not None:
    updates["XRAY_REALITY_PORT"] = str(port)

reality = ((inbound.get("streamSettings") or {}).get("realitySettings") or {})
runtime_private_key = str(reality.get("privateKey") or "").strip()
if not runtime_private_key:
    raise SystemExit("sync-runtime-from-configs: runtime xray reality privateKey is missing in XRAY_SOURCE_CONFIG_PATH")
if runtime_private_key != xray_private_key_env:
    print("sync-runtime-from-configs: xray reality private key mismatch between runtime config and env", file=sys.stderr)
    raise SystemExit("sync-runtime-from-configs: XRAY_REALITY_PRIVATE_KEY does not match runtime config privateKey")

server_names = reality.get("serverNames") or []
short_ids = reality.get("shortIds") or []
if server_names:
    updates["XRAY_REALITY_SERVER_NAME"] = str(server_names[0])
if short_ids:
    updates["XRAY_REALITY_SHORT_ID"] = str(short_ids[0])
if not (env.get("XRAY_PUBLIC_HOST") or "").strip() and server_names:
    updates["XRAY_PUBLIC_HOST"] = str(server_names[0])
updates["XRAY_REALITY_PRIVATE_KEY"] = xray_private_key_env
updates["XRAY_REALITY_PUBLIC_KEY"] = xray_public_key_env

# write back env file
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
