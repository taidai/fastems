#!/usr/bin/env bash
# Production verification: build the Encore image, run it, and exercise the
# same browser checks used for the product. Run in WSL/Linux with Encore,
# Docker, Node and npm available.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT="${EASYEMS_VERIFY_PORT:-4409}"
IMAGE="${EASYEMS_VERIFY_IMAGE:-easyems:verify}"
CONTAINER="easyems-verify-$$"
VOLUME="easyems-verify-data-$$"
EVIDENCE="$ROOT/evidence/verify-encore"
ENV_FILE="$(mktemp)"
PASSWORD_FILE="$(mktemp)"
PASSWORD="${EASYEMS_VERIFY_PASSWORD:-$(openssl rand -hex 16 2>/dev/null || printf 'verify-%s-%s' "$(date +%s)" "$RANDOM")}"
export GOFLAGS="${GOFLAGS:--buildvcs=false}"
if ! git rev-parse --verify HEAD >/dev/null 2>&1; then
  ENCORE_GO="${ENCORE_INSTALL:-$HOME/.encore}/encore-go/bin/go"
  if [ -x "$ENCORE_GO" ]; then
    "$ENCORE_GO" env -w GOFLAGS=-buildvcs=false
  fi
fi

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  docker volume rm "$VOLUME" >/dev/null 2>&1 || true
  rm -f "$ENV_FILE" "$PASSWORD_FILE"
}
trap cleanup EXIT

mkdir -p "$EVIDENCE"
printf 'EASYEMS_ADMIN_PASSWORD=%s\n' "$PASSWORD" >"$ENV_FILE"
printf '账号: admin\n密码: %s\n' "$PASSWORD" >"$PASSWORD_FILE"
chmod 600 "$ENV_FILE" "$PASSWORD_FILE"

echo "== frontend build =="
npm --prefix frontend run build

echo "== Encore image =="
"${ENCORE_BIN:-encore}" build docker --config deploy/encore-infra.json "$IMAGE"

echo "== container on :$PORT =="
docker volume create "$VOLUME" >/dev/null
docker run -d --name "$CONTAINER" \
  -p "$PORT:8080" \
  -v "$VOLUME":/data \
  -e EASYEMS_DATA=/data \
  -e EASYEMS_SEED_DEMO=1 \
  --env-file "$ENV_FILE" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/health"; then
    break
  fi
  sleep 1
done
curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/health"
curl -fsS -o /dev/null "http://127.0.0.1:$PORT/"

echo "== native read-only routes =="
TOKEN="$(curl -fsS -X POST "http://127.0.0.1:$PORT/api/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin\",\"password\":\"$PASSWORD\"}" |
  sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
test -n "$TOKEN"
curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/device-ledger" -H "Authorization: Bearer $TOKEN"
curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/alarm-stats?days=7" -H "Authorization: Bearer $TOKEN"
curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/snapshot" -H "Authorization: Bearer $TOKEN"

echo "== browser smoke =="
export EASYEMS_URL="http://127.0.0.1:$PORT"
export EASYEMS_ADMIN_PASSWORD="$PASSWORD"
export EASYEMS_PASSWORD_FILE="$PASSWORD_FILE"
export EASYEMS_EVIDENCE="$EVIDENCE"
node frontend/browser-smoke.mjs
node frontend/layout-check.mjs
node frontend/theme-check.mjs

echo "Verify (Encore container) passed. Evidence: $EVIDENCE"
