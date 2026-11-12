#!/usr/bin/env bash
# Build the production Encore image. The frontend must be embedded before
# Encore compiles the Go service, because go:embed reads internal/web/dist.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="${FASTEMS_IMAGE:-fastiot:latest}"
ENCORE_BIN="${ENCORE_BIN:-encore}"
# Repositories without a first commit cannot be VCS-stamped.
export GOFLAGS="${GOFLAGS:--buildvcs=false}"
if ! git rev-parse --verify HEAD >/dev/null 2>&1; then
  ENCORE_GO="${ENCORE_INSTALL:-$HOME/.encore}/encore-go/bin/go"
  if [ -x "$ENCORE_GO" ]; then
    "$ENCORE_GO" env -w GOFLAGS=-buildvcs=false
  fi
fi

echo "== frontend build =="
npm --prefix frontend run build

echo "== encore build docker ($IMAGE) =="
"$ENCORE_BIN" build docker --config deploy/encore-infra.json "$IMAGE"

echo "Built $IMAGE"
