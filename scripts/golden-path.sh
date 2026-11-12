#!/usr/bin/env bash
# 黄金路径：启动一个隔离的 FastEMS 实例（独立数据目录、随机管理员密码），
# 按 docs/jobs.md 的 J1–J5 驱动真实界面，输出 evidence/golden-path.json。
# 用法：scripts/golden-path.sh [端口，默认 4412]
set -euo pipefail
# Git Bash 会把 /fastems 改写成 Windows 路径；容器内路径必须原样传递。
export MSYS_NO_PATHCONV=1

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${1:-4412}"
IMAGE="${FASTEMS_IMAGE:-easyems:local}"
BIN="${FASTEMS_BIN:-$ROOT/.tmp-build/fastems}"
NAME="fastems-golden-$PORT"
DATA="$ROOT/.tmp-golden/$PORT"

if [ ! -s "$BIN" ]; then
  echo "binary not found or empty: $BIN (run the Go build first)" >&2
  exit 1
fi

PASSWORD="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
rm -rf "$DATA"
mkdir -p "$DATA"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d --name "$NAME" \
  -p "127.0.0.1:$PORT:4412" \
  -v "$DATA:/data" \
  -v "$BIN:/fastems:ro" \
  -e FASTEMS_ADMIN_PASSWORD="$PASSWORD" \
  -e FASTEMS_SEED_DEMO=1 \
  --entrypoint /fastems \
  "$IMAGE" -port 4412 -data /data >/dev/null

ready=0
for _ in $(seq 1 90); do
  if [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/health" || true)" = "200" ]; then
    ready=1
    break
  fi
  sleep 1
done
if [ "$ready" != "1" ]; then
  echo "isolated instance did not become ready on port $PORT" >&2
  docker logs "$NAME" 2>&1 | tail -20 >&2 || true
  exit 1
fi

cd "$ROOT/frontend"
FASTEMS_GOLDEN_URL="http://127.0.0.1:$PORT" FASTEMS_GOLDEN_PASSWORD="$PASSWORD" node golden-path.mjs
