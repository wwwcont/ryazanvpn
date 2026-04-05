#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "[boundary-check] ERROR: $*" >&2
  exit 1
}

# Control-plane must not import node http transport package.
if rg -n 'internal/transport/httpnode' cmd/control-plane internal/transport/httpcontrol >/dev/null; then
  fail "control-plane layer imports node-agent http transport package"
fi

# Freeze-window guard: control-plane side must not import node-agent runtime/shell internals.
if rg -n 'internal/agent/(runtime|shell)' cmd/control-plane internal/transport/httpcontrol internal/app >/dev/null; then
  fail "control-plane layer imports node-agent runtime/shell package"
fi

# Shared contracts package must remain dependency-light (no internal imports).
if rg -n 'internal/' shared/contracts >/dev/null; then
  fail "shared/contracts must not import internal/*"
fi

echo "[boundary-check] OK"
