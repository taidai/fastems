#!/usr/bin/env bash
# Legacy single-binary verification. Production verification is
# scripts/verify-encore.sh; see docs/encore-migration.md.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT="${FASTEMS_VERIFY_PORT:-62971}"
NATS_PORT="${FASTEMS_VERIFY_NATS_PORT:-9851}"
DATA="$ROOT/data/verify"
EVIDENCE="$ROOT/evidence/verify"
LOG="$EVIDENCE/server.log"
PID=""

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) BIN="fastems-verify.exe"; GOOS=windows ;;
  *) BIN="fastems-verify"; GOOS=linux ;;
esac

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; fi
}
trap cleanup EXIT

rm -rf "$DATA" "$EVIDENCE"
mkdir -p "$DATA" "$EVIDENCE"

echo "== frontend build =="
npm --prefix frontend run build

echo "== Go test / vet / build =="
if command -v go >/dev/null 2>&1; then
  GOOS="$GOOS" go test ./...
  go vet ./...
  GOOS="$GOOS" go build -trimpath -o "$BIN" ./cmd/fastems
else
  MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT":/src -v fastems-gomod:/go/pkg/mod -w /src -e CGO_ENABLED=0 golang:1.27.3 go test ./...
  MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT":/src -v fastems-gomod:/go/pkg/mod -w /src -e CGO_ENABLED=0 golang:1.27.3 go vet ./...
  MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT":/src -v fastems-gomod:/go/pkg/mod -w /src -e CGO_ENABLED=0 -e GOOS="$GOOS" -e GOARCH=amd64 golang:1.27.3 go build -trimpath -o "$BIN" ./cmd/fastems
fi

echo "== temp instance on :$PORT =="
FASTEMS_SEED_DEMO=1 "./$BIN" -port "$PORT" -nats-port "$NATS_PORT" -data "$DATA" >"$LOG" 2>&1 &
PID=$!
curl -s --retry 30 --retry-delay 1 --retry-connrefused -o /dev/null -w "health %{http_code}\n" "http://127.0.0.1:$PORT/api/health"

PASSWORD_FILE="$DATA/initial-password.txt"
export FASTEMS_URL="http://127.0.0.1:$PORT"
export FASTEMS_DATA="$DATA"
export FASTEMS_PASSWORD_FILE="$PASSWORD_FILE"
export FASTEMS_EVIDENCE="$EVIDENCE"

echo "== browser smoke =="
node frontend/browser-smoke.mjs
echo "== layout =="
node frontend/layout-check.mjs
echo "== theme =="
node frontend/theme-check.mjs

echo "Verify passed. Evidence: $EVIDENCE"
cleanup
sleep 1
rm -rf "$DATA" || true
rm -f "$BIN" || true
