#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cp -R "$ROOT_DIR"/. "$TMP_DIR"/
cd "$TMP_DIR"

# Seed minimal runtime configs for auto-detect.
mkdir -p deploy/node/amnezia deploy/node/xray
cat > deploy/node/amnezia/awg0.conf <<CFG
[Interface]
PrivateKey = test_amnezia_private_key
ListenPort = 41475
CFG
cat > deploy/node/xray/config.json <<JSON
{"inbounds":[{"port":8443,"streamSettings":{"realitySettings":{"privateKey":"xray_priv_key","serverNames":["www.cloudflare.com"],"shortIds":["0123456789abcdef"]}}}]}
JSON

# 1) validate-only should fail in non-interactive mode if required vars are missing.
set +e
BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --env-file "$TMP_DIR/missing.env" --non-interactive --validate-only >/tmp/bootstrap_missing.log 2>&1
rc=$?
set -e
if [[ $rc -eq 0 ]]; then
  echo "expected failure for missing required env values"
  exit 1
fi

# 2) Happy path in non-interactive mode with complete env should pass.
cp deploy/env/node.env.example "$TMP_DIR/good.env"
cat >> "$TMP_DIR/good.env" <<ENV
NODE_ID=node-test-1
NODE_TOKEN=token-test-1
CONTROL_PLANE_BASE_URL=http://control-plane:8080
AGENT_HMAC_SECRET=hmac-secret-1
NODE_AGENT_HMAC_SECRET=hmac-secret-1
AMNEZIA_CONTAINER_NAME=amnezia-awg2
AMNEZIA_INTERFACE_NAME=awg0
XRAY_CONTAINER_NAME=ryazanvpn-xray
XRAY_REALITY_PRIVATE_KEY=xray_priv_key
ENV

BOOTSTRAP_SKIP_DOCKER=1 ./scripts/bootstrap-node.sh --env-file "$TMP_DIR/good.env" --non-interactive --validate-only

grep -q '^NODE_ID=node-test-1$' "$TMP_DIR/good.env"
grep -q '^XRAY_REALITY_PRIVATE_KEY=xray_priv_key$' "$TMP_DIR/good.env"

echo "bootstrap-node shell integration tests passed"
