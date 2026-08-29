#!/usr/bin/env bash
set -euo pipefail

# Per-package budget for the race gate. Default 900s: handler/admin -race was
# measured at 217-364s on contended dev hosts (8GB WSL VM, /mnt/d 9p) across
# six Wave 18 lanes, and CI gives each package ~10 min (Go's default test
# timeout). Override with METAPI_RACE_TIMEOUT_SECONDS=<seconds>; see
# docs/testing.md "Race-detector budget".
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

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    if ! command -v wsl.exe >/dev/null 2>&1; then
      echo "race: WSL is required on Windows because the native race runtime may fail to reserve its address space." >&2
      exit 1
    fi
    windows_repo="$(pwd -W)"
    echo "race: Windows detected; running the Go race detector inside WSL"
    export MSYS_NO_PATHCONV=1
    run_race wsl.exe --cd "$windows_repo" -- \
      bash -lc "go test ./... -count=1 -race -timeout ${timeout_seconds}s"
    ;;
  *)
    run_race go test ./... -count=1 -race -timeout "${timeout_seconds}s"
    ;;
esac
