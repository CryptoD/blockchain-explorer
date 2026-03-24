#!/usr/bin/env bash
# Validate Prometheus alert rules if promtool is installed (Prometheus distribution).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RULES="${ROOT}/prometheus/prometheus-alerts.yml"
if ! command -v promtool >/dev/null 2>&1; then
  echo "promtool not found; install Prometheus or add promtool to PATH."
  echo "Skipping validation."
  exit 0
fi
promtool check rules "${RULES}"
echo "OK: ${RULES}"
