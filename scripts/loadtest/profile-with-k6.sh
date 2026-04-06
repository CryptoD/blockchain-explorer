#!/usr/bin/env bash
# Capture CPU and heap profiles while running the production-like k6 scenarios.
#
# Prerequisites: server running with GETBLOCK_* and Redis; k6 installed; curl.
# Enable pprof (off by default for security):
#   PPROF_ENABLED=true go run ./cmd/server
#
# Usage:
#   BASE_URL=http://127.0.0.1:8080 \
#   LOADTEST_USERNAME=admin LOADTEST_PASSWORD=admin123 \
#   ./scripts/loadtest/profile-with-k6.sh [output_dir]
#
# Writes heap.pb.gz, cpu.pb.gz, and k6-summary.txt under output_dir (default: ./profiles).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
OUT="${1:-./profiles}"
mkdir -p "$OUT"

hostport="${BASE_URL#http://}"
hostport="${hostport#https://}"
PPROF_BASE="${BASE_URL}/debug/pprof"

echo "Checking ${PPROF_BASE}/ — set PPROF_ENABLED=true on the server if this fails."
curl -sfS "${PPROF_BASE}/" -o /dev/null

echo "Starting 30s CPU profile → ${OUT}/cpu.pb.gz"
curl -sfS "${PPROF_BASE}/profile?duration=30s" -o "${OUT}/cpu.pb.gz" &
CPU_PID=$!

sleep 2

echo "Running k6 (scripts/loadtest/k6.js)..."
k6 run --summary-export "${OUT}/k6-summary.json" scripts/loadtest/k6.js \
  | tee "${OUT}/k6-summary.txt" || true

wait "$CPU_PID" || true

echo "Heap profile → ${OUT}/heap.pb.gz"
curl -sfS "${PPROF_BASE}/heap" -o "${OUT}/heap.pb.gz"

echo "Done. Inspect with:"
echo "  go tool pprof -http=:0 ${OUT}/heap.pb.gz"
echo "  go tool pprof -http=:0 ${OUT}/cpu.pb.gz"
