#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "[boundary-check] ERROR: $*" >&2
  exit 1
}

# Node-agent must not import control-plane transport package.
if rg -n 'internal/transport/httpcontrol' cmd/node-agent internal/agent >/dev/null; then
  fail "node-agent layer imports control-plane transport package"
fi

# Control-plane must not import node http transport package.
if rg -n 'internal/transport/httpnode' cmd/control-plane internal/transport/httpcontrol >/dev/null; then
  fail "control-plane layer imports node-agent http transport package"
fi

# Shared contracts package must remain dependency-light (no internal imports).
if rg -n 'internal/' shared/contracts >/dev/null; then
  fail "shared/contracts must not import internal/*"
fi

echo "[boundary-check] OK"
