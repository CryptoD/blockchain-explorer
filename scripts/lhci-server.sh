#!/usr/bin/env bash
# Starts the explorer for Lighthouse CI; prints "Server is up" when /health responds.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export APP_ENV="${APP_ENV:-development}"
export REDIS_HOST="${REDIS_HOST:-localhost}"
export GETBLOCK_BASE_URL="${GETBLOCK_BASE_URL:-https://example.invalid/}"
export GETBLOCK_ACCESS_TOKEN="${GETBLOCK_ACCESS_TOKEN:-lhci-token}"
export HTTP_LISTEN_ADDR="${HTTP_LISTEN_ADDR:-127.0.0.1:18080}"

if [[ ! -x ./explorer ]]; then
  echo "Missing ./explorer binary; run: go build -o explorer ./cmd/server" >&2
  exit 1
fi

./explorer &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT INT TERM

for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:18080/health" > /dev/null; then
    echo "Server is up"
    wait "$SERVER_PID"
    exit 0
  fi
  sleep 1
done

echo "Server failed to become healthy" >&2
exit 1
