#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/data/dev"
export EASYEMS_API="${EASYEMS_API:-http://127.0.0.1:62943}"
if command -v air >/dev/null 2>&1; then
  (cd "$ROOT" && air) &
else
  echo "air not found; starting existing easyems.exe (Go changes need a manual rebuild)."
  (cd "$ROOT" && ./easyems.exe -port 62943 -nats-port 9820 -data "$ROOT/data/dev") &
fi
trap 'kill $! 2>/dev/null || true' EXIT
cd "$ROOT/frontend"
npm run dev
