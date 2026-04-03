#!/usr/bin/env bash
# Enforce minimum per-package statement coverage from coverage_thresholds.txt.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
THRESH="${ROOT}/scripts/coverage_thresholds.txt"
cd "$ROOT"

if [[ ! -f "$THRESH" ]]; then
  echo "missing $THRESH" >&2
  exit 1
fi

fail=0
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "${line// }" ]] && continue
  [[ "$line" =~ ^[[:space:]]*# ]] && continue

  pkg="${line%%[[:space:]]*}"
  min="${line##*[[:space:]]}"

  if [[ -z "$pkg" || -z "$min" || "$pkg" == "$min" ]]; then
    echo "bad line in $THRESH: $line" >&2
    exit 1
  fi

  out="$(go test -cover "$pkg" -covermode=atomic 2>&1)" || true

  if echo "$out" | grep -q '\[no test files\]'; then
    echo "::error::coverage check: $pkg has no test files but is listed in coverage_thresholds.txt"
    fail=1
    continue
  fi

  pct="$(echo "$out" | sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p' | tail -1)"
  if [[ -z "$pct" ]]; then
    echo "::error::coverage check: could not parse coverage for $pkg"
    echo "$out" >&2
    fail=1
    continue
  fi

  # awk compares floats: exit 0 if pct >= min
  if awk -v p="$pct" -v m="$min" 'BEGIN { exit (p + 0 >= m + 0) ? 0 : 1 }'; then
    echo "ok   $pkg  ${pct}%  (min ${min}%)"
  else
    echo "::error::coverage check: $pkg ${pct}% is below minimum ${min}%"
    fail=1
  fi
done < "$THRESH"

exit "$fail"
