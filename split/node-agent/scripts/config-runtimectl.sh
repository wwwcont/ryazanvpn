#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

SUBCOMMAND="${1:-}"; shift || true
ENV_FILE=".env"
DRY_RUN="0"

log() { echo "[config-runtimectl] $*"; }
fail() { echo "[config-runtimectl] ERROR: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) shift; [[ $# -gt 0 ]] || fail "--env-file requires value"; ENV_FILE="$1" ;;
    --dry-run) DRY_RUN="1" ;;
    *) fail "unknown option: $1" ;;
  esac
  shift
done

[[ -n "$SUBCOMMAND" ]] || fail "subcommand required: validate-config|render-config|inspect-runtime|reconcile-runtime"
[[ -f "$ENV_FILE" ]] || fail "env file not found: $ENV_FILE"

PY="$(cat <<'PYEOF'
import base64,hashlib,json,os,re,subprocess,sys
from pathlib import Path

cmd, env_path, dry_run = sys.argv[1], Path(sys.argv[2]), sys.argv[3] == "1"
pat = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$")
vals = {}
for line in env_path.read_text(encoding="utf-8").splitlines():
    m = pat.match(line.strip())
    if m:
        vals[m.group(1)] = m.group(2)

placeholders = {
  "NODE_ID": {"single-node-1"},
  "NODE_TOKEN": {"52esUBVD9Rr4hXoPE5QzlOpdgJEfUR+xeEMTHh7YSuc="},
  "AGENT_HMAC_SECRET": {"xG6yzhNAXO9BG77BYBD7g3DgvxahDdYm1EqztcMHGs8="},
  "NODE_AGENT_HMAC_SECRET": {"xG6yzhNAXO9BG77BYBD7g3DgvxahDdYm1EqztcMHGs8="},
  "XRAY_REALITY_SHORT_ID": {"0123456789abcdef"},
}

required = ["NODE_ID","NODE_TOKEN","CONTROL_PLANE_BASE_URL","AGENT_HMAC_SECRET","AMNEZIA_CONTAINER_NAME","AMNEZIA_INTERFACE_NAME","AMNEZIA_PRIVATE_KEY","AMNEZIA_PORT","XRAY_CONTAINER_NAME","XRAY_REALITY_PRIVATE_KEY","XRAY_REALITY_PUBLIC_KEY","XRAY_REALITY_PORT","XRAY_REALITY_SERVER_NAME","XRAY_REALITY_SHORT_ID","XRAY_PUBLIC_HOST","VPN_SERVER_PUBLIC_ENDPOINT","VPN_SUBNET_CIDR"]

errors = []
for k in required:
    if not vals.get(k,"").strip():
        errors.append(f"missing required env: {k}")

for k,cands in placeholders.items():
    if vals.get(k,"").strip() in cands:
        errors.append(f"placeholder value from example is not allowed for {k}")

try:
    p = int(vals.get("AMNEZIA_PORT","0")); assert 1 <= p <= 65535
except Exception:
    errors.append("AMNEZIA_PORT must be valid port 1..65535")
try:
    p = int(vals.get("XRAY_REALITY_PORT","0")); assert 1 <= p <= 65535
except Exception:
    errors.append("XRAY_REALITY_PORT must be valid port 1..65535")

if not re.match(r"^[0-9a-fA-F]{16}$", vals.get("XRAY_REALITY_SHORT_ID","")):
    errors.append("XRAY_REALITY_SHORT_ID must be 16 hex chars")
if not re.match(r"^[A-Za-z0-9._-]+$", vals.get("XRAY_REALITY_SERVER_NAME","")):
    errors.append("XRAY_REALITY_SERVER_NAME must be a hostname-like value")
if not re.match(r"^[A-Za-z0-9._-]+$", vals.get("XRAY_PUBLIC_HOST","")):
    errors.append("XRAY_PUBLIC_HOST must be a hostname-like value")

for key in ("AGENT_HMAC_SECRET","NODE_AGENT_HMAC_SECRET","AMNEZIA_PRIVATE_KEY","VPN_SERVER_PUBLIC_KEY"):
    if vals.get(key,""):
        try: base64.b64decode(vals[key], validate=True)
        except Exception: errors.append(f"{key} must be valid base64")

for key in ("XRAY_REALITY_PRIVATE_KEY","XRAY_REALITY_PUBLIC_KEY"):
    if vals.get(key,""):
        if not re.match(r"^[A-Za-z0-9_-]{43,44}$", vals[key]):
            errors.append(f"{key} must be base64url-like (43-44 chars)")

if not re.match(r"^[A-Za-z0-9._-]+:\d+$", vals.get("VPN_SERVER_PUBLIC_ENDPOINT","")):
    errors.append("VPN_SERVER_PUBLIC_ENDPOINT must be host:port")
if not re.match(r"^\d+\.\d+\.\d+\.\d+/\d+$", vals.get("VPN_SUBNET_CIDR","")):
    errors.append("VPN_SUBNET_CIDR must be CIDR (ipv4)")

if errors:
    for err in errors: print(f"config.validate.error {err}", file=sys.stderr)
    raise SystemExit(2)
print(f"config.validate.ok env_file={env_path}")

am_conf = Path("deploy/node/amnezia/awg0.conf")
xray_conf = Path("deploy/node/xray/config.json")

def sha(path):
    if not path.exists(): return ""
    return hashlib.sha256(path.read_bytes()).hexdigest()

if cmd == "validate-config":
    raise SystemExit(0)

if cmd == "render-config":
    before_a, before_x = sha(am_conf), sha(xray_conf)
    if dry_run:
        print(f"config.render.dry_run amnezia_before={before_a} xray_before={before_x}")
        raise SystemExit(0)
    subprocess.check_call(["./scripts/ensure-amnezia-server-config.sh", str(env_path), str(am_conf)])
    subprocess.check_call(["./scripts/ensure-xray-server-config.sh", str(env_path), str(xray_conf)])
    after_a, after_x = sha(am_conf), sha(xray_conf)
    print(json.dumps({"event":"config.render.done","actor":"config-runtimectl","env_file":str(env_path),"amnezia_hash_before":before_a,"amnezia_hash_after":after_a,"xray_hash_before":before_x,"xray_hash_after":after_x}, ensure_ascii=False))
    raise SystemExit(0)

if cmd in ("inspect-runtime", "reconcile-runtime"):
    docker = (vals.get("DOCKER_BINARY_PATH") or "docker").strip() or "docker"
    am_container = vals.get("AMNEZIA_CONTAINER_NAME","").strip()
    iface = vals.get("AMNEZIA_INTERFACE_NAME","").strip()
    xray_container = vals.get("XRAY_CONTAINER_NAME","").strip()
    drift = []
    def run(args):
        p = subprocess.run(args, text=True, capture_output=True)
        return p.returncode, (p.stdout or "").strip(), (p.stderr or "").strip()
    rc,out,err = run([docker, "exec", am_container, "awg", "show", iface, "listen-port"])
    if rc != 0: drift.append(f"amnezia_runtime_unavailable:{err or out}")
    elif out != vals.get("AMNEZIA_PORT","").strip(): drift.append(f"amnezia_port:{out}!={vals.get('AMNEZIA_PORT','').strip()}")
    rc,out,err = run([docker, "exec", xray_container, "cat", vals.get("XRAY_SOURCE_CONFIG_PATH") or "/opt/xray/config.json"])
    if rc == 0:
        try:
            cfg = json.loads(out)
            inb=(cfg.get("inbounds") or [{}])[0]
            r=((inb.get("streamSettings") or {}).get("realitySettings") or {})
            if str(inb.get("port") or "") != vals.get("XRAY_REALITY_PORT","").strip():
                drift.append("xray_port_mismatch")
            if str(r.get("privateKey") or "").strip() != vals.get("XRAY_REALITY_PRIVATE_KEY","").strip():
                drift.append("xray_private_key_mismatch")
        except Exception as e:
            drift.append(f"xray_runtime_parse_error:{e}")
    else:
        drift.append(f"xray_runtime_unavailable:{err or out}")

    if cmd == "reconcile-runtime" and not dry_run:
        subprocess.check_call(["./scripts/ensure-amnezia-server-config.sh", str(env_path), str(am_conf)])
        subprocess.check_call(["./scripts/ensure-xray-server-config.sh", str(env_path), str(xray_conf)])
    print(json.dumps({"event":"runtime.inspect","drift":drift,"drift_detected":len(drift)>0,"mode":cmd}, ensure_ascii=False))
    raise SystemExit(1 if drift else 0)

raise SystemExit(f"unknown subcommand: {cmd}")
PYEOF
)"

python3 -c "$PY" "$SUBCOMMAND" "$ENV_FILE" "$DRY_RUN"
