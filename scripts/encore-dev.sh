#!/usr/bin/env bash
# Start the Encore development runtime. Run this from WSL/Linux with the
# Encore CLI on PATH; the old single-binary dev loop remains in dev.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export FASTEMS_DATA="${FASTEMS_DATA:-$ROOT/data/encore}"
export GOFLAGS="${GOFLAGS:--buildvcs=false}"
PORT="${FASTEMS_PORT:-4000}"

exec encore run --port "$PORT" "$@"
