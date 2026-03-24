#!/usr/bin/env bash
# Run the main package tests against a real Redis (Docker).
# Default unit tests use in-process miniredis and do not need this.
set -euo pipefail
port="${TEST_REDIS_PORT:-6379}"
cid="$(docker run --rm -d -p "${port}:6379" redis:7-alpine)"
cleanup() { docker stop "${cid}" >/dev/null 2>&1 || true; }
trap cleanup EXIT
sleep 1
export BLOCKCHAIN_EXPLORER_TEST_REDIS=integration
export TEST_REDIS_ADDR="127.0.0.1:${port}"
go test ./... -count=1 -short
