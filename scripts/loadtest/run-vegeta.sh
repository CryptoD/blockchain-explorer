#!/usr/bin/env bash
# Vegeta load test (GET paths). For POST /api/v1/login use k6 (scripts/loadtest/k6.js).
# Install: https://github.com/tsenart/vegeta/releases
set -euo pipefail
HOST="${BASE_URL:-http://127.0.0.1:8080}"
RATE="${VEGETA_RATE:-20}"
DURATION="${VEGETA_DURATION:-60s}"

if ! command -v vegeta >/dev/null 2>&1; then
  echo "vegeta not found; install from https://github.com/tsenart/vegeta/releases" >&2
  exit 1
fi

targets="$(mktemp)"
trap 'rm -f "$targets"' EXIT

cat >"$targets" <<EOF
GET ${HOST}/api/search?q=1
GET ${HOST}/api/v1/search/advanced?page=1&page_size=50&sort_by=rank&sort_dir=asc
GET ${HOST}/dashboard
EOF

echo "BASE_URL=${HOST} vegeta attack -rate=${RATE}/1s -duration=${DURATION}"
vegeta attack -rate="${RATE}/1s" -duration="${DURATION}" -targets="$targets" | vegeta report
