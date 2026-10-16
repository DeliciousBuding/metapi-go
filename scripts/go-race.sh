#!/usr/bin/env bash
set -euo pipefail

# Match CI's per-package budget; override for slower local machines.
# See docs/testing.md "Race-detector budget".
timeout_seconds="${METAPI_RACE_TIMEOUT_SECONDS:-900}"
if [[ ! "$timeout_seconds" =~ ^[0-9]+$ ]] || (( timeout_seconds < 1 )); then
  echo "race: METAPI_RACE_TIMEOUT_SECONDS must be a positive integer." >&2
  exit 2
fi

race_log="$(mktemp)"
trap 'rm -f "$race_log"' EXIT

# Stream the race run while keeping a copy; if it fails on a per-package
# timeout, point the operator at the budget knob.
run_race() {
  if "$@" 2>&1 | tee "$race_log"; then
    return 0
  fi
  if grep -q "test timed out" "$race_log"; then
    echo "race: a package exceeded the ${timeout_seconds}s per-package budget; raise it with METAPI_RACE_TIMEOUT_SECONDS=<seconds>." >&2
  fi
  return 1
}

# Run in the caller's environment; WSL/container use is an explicit choice.
run_race go test ./... -count=1 -race -timeout "${timeout_seconds}s"
