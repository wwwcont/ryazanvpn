#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cp -R "$ROOT_DIR"/. "$TMP_DIR"/
cd "$TMP_DIR"

mkdir -p deploy/node/amnezia deploy/node/xray
cat > deploy/node/xray/config.json <<JSON
{"inbounds":[{"port":8443,"streamSettings":{"realitySettings":{"privateKey":"abcdEFGHijklMNOPqrstUVWXyz0123456789abc","serverNames":["example.com"],"shortIds":["aabbccddeeff0011"]}}}]}
JSON

cat > good.env <<ENV
NODE_ID=node-test-1
NODE_TOKEN=token-test-1
CONTROL_PLANE_BASE_URL=http://control-plane:8080
AGENT_HMAC_SECRET=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo0NTY3ODkwMTIzNDU2Nzg=
NODE_AGENT_HMAC_SECRET=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo0NTY3ODkwMTIzNDU2Nzg=
AMNEZIA_CONTAINER_NAME=amnezia-awg2
AMNEZIA_INTERFACE_NAME=awg0
AMNEZIA_PRIVATE_KEY=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo0NTY3ODkwMTIzNDU2Nzg=
AMNEZIA_PORT=41475
XRAY_CONTAINER_NAME=ryazanvpn-xray
XRAY_SOURCE_CONFIG_PATH=/opt/xray/config.json
XRAY_REALITY_PRIVATE_KEY=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi01234567_
XRAY_REALITY_PUBLIC_KEY=ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi01234567_
XRAY_REALITY_PORT=8443
XRAY_REALITY_SERVER_NAME=example.com
XRAY_REALITY_SHORT_ID=aabbccddeeff0011
XRAY_PUBLIC_HOST=example.com
VPN_SERVER_PUBLIC_ENDPOINT=vpn.example.com:41475
VPN_SUBNET_CIDR=10.8.1.0/24
ENV

# 1) validate-config should pass.
./scripts/config-runtimectl.sh validate-config --env-file good.env

# 2) bootstrap validate-only should not mutate env.
cp good.env before.env
BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --env-file good.env --non-interactive --validate-only
cmp -s good.env before.env

# 3) repeated bootstrap is idempotent.
BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --env-file good.env --non-interactive --validate-only

# 4) example leakage is blocked.
cp deploy/env/node.env.example bad.env
set +e
./scripts/config-runtimectl.sh validate-config --env-file bad.env >/tmp/bad.log 2>&1
rc=$?
set -e
[[ $rc -ne 0 ]]

echo "bootstrap/config runtime shell integration tests passed"
