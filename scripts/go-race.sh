#!/usr/bin/env bash
set -euo pipefail

# Nonempty whitespace overrides GOFLAGS saved with go env -w and parses as no
# flags. An empty value would fall back to the saved configuration. Selectors
# (-run/-skip/-short) or -exec must not turn a full gate into a partial/no-op run.
export GOFLAGS=" "

# Keep the full local gate bounded on developer machines. CI owns its own
# runner/shard budget; these controls do not remove any packages or tests.
if (( $# > 0 )); then
  if [[ $# == 1 && "$1" == "--help" ]]; then
    echo "Usage: bash scripts/go-race.sh"
    echo "Runs every Go package with -race and -count=1 in the current environment."
    echo "GOMAXPROCS=2                    Go execution parallelism per process"
    echo "METAPI_RACE_PACKAGES=2           Concurrent build/test packages (-p)"
    echo "METAPI_RACE_PARALLEL=2           Parallel tests per package (-parallel)"
    echo "METAPI_RACE_TIMEOUT_SECONDS=900  Per-package timeout"
    echo "With PG_TEST_DSN set, package parallelism defaults to 1 and must stay 1."
    echo "On Windows, run this script from WSL if ThreadSanitizer cannot allocate shadow memory."
    exit 0
  fi
  echo "race: unexpected arguments; use --help. Use go test directly for focused tests." >&2
  exit 2
fi

timeout_seconds="${METAPI_RACE_TIMEOUT_SECONDS:-900}"
default_packages=2
if [[ -n "${PG_TEST_DSN:-}" ]]; then
  default_packages=1
fi
package_parallelism="${METAPI_RACE_PACKAGES:-$default_packages}"
test_parallelism="${METAPI_RACE_PARALLEL:-2}"
export GOMAXPROCS="${GOMAXPROCS:-2}"
for name in timeout_seconds package_parallelism test_parallelism GOMAXPROCS; do
  if [[ ! "${!name}" =~ ^[1-9][0-9]*$ ]]; then
    echo "race: $name must be a positive integer without leading zeros." >&2
    exit 2
  fi
done
if [[ -n "${PG_TEST_DSN:-}" && "$package_parallelism" != 1 ]]; then
  echo "race: shared PG_TEST_DSN fixtures require METAPI_RACE_PACKAGES=1." >&2
  exit 2
fi

echo "race: full suite; GOMAXPROCS=$GOMAXPROCS, -p=$package_parallelism, -parallel=$test_parallelism, timeout=${timeout_seconds}s"

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
  if grep -qE "ThreadSanitizer failed to allocate|error code: 87" "$race_log"; then
    echo "race: Windows native ThreadSanitizer could not allocate its shadow memory." >&2
    echo "race: run the same gate from WSL:" >&2
    wsl_repo=""
    if command -v wslpath >/dev/null 2>&1; then
      wsl_repo="$(wslpath -a . 2>/dev/null || true)"
    fi
    echo "  wsl --cd '${wsl_repo:-/path/to/metapi-go}' -- bash -lc 'bash scripts/go-race.sh'" >&2
  fi
  return 1
}

# Run in the caller's environment; WSL/container use is an explicit choice.
if ! run_race go test ./... -count=1 -race -p "$package_parallelism" -parallel "$test_parallelism" -timeout "${timeout_seconds}s"; then
  exit 1
fi
