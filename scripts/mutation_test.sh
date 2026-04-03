#!/usr/bin/env bash
# Optional mutation testing (ROADMAP task 27) using Gremlins.
# Install: go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1
# Run from repo root: ./scripts/mutation_test.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GREMLINS_BIN="${GREMLINS:-}"
if [[ -z "$GREMLINS_BIN" ]] && command -v gremlins >/dev/null 2>&1; then
  GREMLINS_BIN="$(command -v gremlins)"
fi
if [[ -z "$GREMLINS_BIN" ]]; then
  GOBIN="$(go env GOBIN)"
  GOPATH="$(go env GOPATH)"
  if [[ -n "$GOBIN" && -x "$GOBIN/gremlins" ]]; then
    GREMLINS_BIN="$GOBIN/gremlins"
  elif [[ -x "$GOPATH/bin/gremlins" ]]; then
    GREMLINS_BIN="$GOPATH/bin/gremlins"
  fi
fi

if [[ -z "$GREMLINS_BIN" || ! -x "$GREMLINS_BIN" ]]; then
  echo "Gremlins not found. Install with:" >&2
  echo "  go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1" >&2
  exit 1
fi

WORKERS="${GREMLINS_WORKERS:-4}"
TIMEOUT_COEFF="${GREMLINS_TIMEOUT_COEFF:-4}"

# Small, mostly pure packages; exclude fuzz tests (slow / different goals).
PACKAGES=(
  ./internal/apperrors
  ./internal/correlation
  ./internal/apiutil
)

for pkg in "${PACKAGES[@]}"; do
  echo ""
  echo "=== gremlins unleash $pkg ==="
  "$GREMLINS_BIN" unleash "$pkg" \
    --timeout-coefficient "$TIMEOUT_COEFF" \
    --workers "$WORKERS" \
    -E 'fuzz_test\.go'
done

echo ""
echo "Done. Review LIVED / NOT COVERED lines to strengthen tests."
