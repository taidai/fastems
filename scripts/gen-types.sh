#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT":/src -w /src golang:1.27.3 \
  sh -c 'GOFLAGS=-mod=mod go run github.com/gzuidhof/tygo@v0.2.21 generate'
echo "Generated frontend/src/generated/contract.ts"
