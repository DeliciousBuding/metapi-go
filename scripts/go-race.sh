#!/usr/bin/env bash
set -euo pipefail

timeout_seconds="${METAPI_RACE_TIMEOUT_SECONDS:-300}"
if [[ ! "$timeout_seconds" =~ ^[0-9]+$ ]] || (( timeout_seconds < 1 )); then
  echo "race: METAPI_RACE_TIMEOUT_SECONDS must be a positive integer." >&2
  exit 2
fi

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    if ! command -v wsl.exe >/dev/null 2>&1; then
      echo "race: WSL is required on Windows because the native race runtime may fail to reserve its address space." >&2
      exit 1
    fi
    windows_repo="$(pwd -W)"
    echo "race: Windows detected; running the Go race detector inside WSL"
    MSYS_NO_PATHCONV=1 wsl.exe --cd "$windows_repo" -- \
      bash -lc "go test ./... -count=1 -race -timeout ${timeout_seconds}s"
    ;;
  *)
    go test ./... -count=1 -race -timeout "${timeout_seconds}s"
    ;;
esac
