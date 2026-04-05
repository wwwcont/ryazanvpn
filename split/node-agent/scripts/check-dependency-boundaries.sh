#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "[boundary-check] ERROR: $*" >&2
  exit 1
}

# Node-agent must not import control-plane transport package.
if rg -n 'internal/transport/httpcontrol' cmd/node-agent internal/agent internal/app >/dev/null; then
  fail "node-agent layer imports control-plane transport package"
fi

# Node-agent repo must not contain control-plane entrypoint imports by mistake.
if [ -d "cmd/control-plane" ]; then
  fail "unexpected cmd/control-plane directory in node-agent split"
fi

# Shared contracts package must remain dependency-light (no internal imports).
if rg -n 'internal/' shared/contracts >/dev/null; then
  fail "shared/contracts must not import internal/*"
fi

echo "[boundary-check] OK"
