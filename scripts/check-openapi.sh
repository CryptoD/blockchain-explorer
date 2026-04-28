#!/usr/bin/env bash
# Validate repository root openapi.yaml (OpenAPI 3) using Redocly CLI in Docker (CI-friendly).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! docker info >/dev/null 2>&1; then
  echo "::error::Docker required for openapi validation (redocly/cli image)." >&2
  exit 1
fi

echo "Linting openapi.yaml..."
docker run --rm \
  -w /spec \
  -v "${ROOT}:/spec:ro" \
  redocly/cli:latest lint openapi.yaml --config .redocly.yaml
