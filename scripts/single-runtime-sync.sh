#!/usr/bin/env bash
set -euo pipefail

exec ./scripts/runtime-sync-env.sh "${1:-.env.single.generated}"
