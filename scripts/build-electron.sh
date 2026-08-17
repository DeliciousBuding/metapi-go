#!/usr/bin/env bash
# Build the Metapi Electron desktop shell.
#
# Produces:
#   - electron/metapi (or electron/metapi.exe)  — the Go server binary
#   - electron/dist/<platform>-<arch>/…          — the packaged desktop app
#
# Prerequisites: Go toolchain, Node.js >= 18, npm. If web/dist/ is missing,
# the script tries `make web-build` (which requires Bun); otherwise it errors.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

ROOT="$(pwd)"
ELECTRON_DIR="$ROOT/electron"
# Go automatically appends ".exe" on Windows when the output name has no
# extension, so we always pass "metapi" and let the toolchain do the right thing.
GO_OUT="$ELECTRON_DIR/metapi"

echo "==> [1/4] ensure web/dist exists (Go embeds it)"
if [ ! -d "$ROOT/web/dist" ]; then
  echo "    web/dist missing — building frontend"
  if command -v bun >/dev/null 2>&1; then
    make web-build
  else
    echo "    ERROR: web/dist is missing and bun is not installed." >&2
    echo "    Build the SPA first with: cd web && bun install && bun run build:web" >&2
    exit 1
  fi
else
  echo "    web/dist present"
fi

echo "==> [2/4] build the Go server binary -> electron/"
go build -trimpath -o "$GO_OUT" ./cmd/server
echo "    built: $GO_OUT"

echo "==> [3/4] install Electron dependencies"
(
  cd "$ELECTRON_DIR"
  if [ ! -d node_modules ]; then
    npm install
  else
    echo "    node_modules present, skipping npm install"
  fi
)

echo "==> [4/4] package the desktop app with electron-packager"
(
  cd "$ELECTRON_DIR"
  npm run package
)

echo ""
echo "Done. Packaged app is in electron/dist/."
