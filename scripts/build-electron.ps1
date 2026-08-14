# Build the MetAPI Electron desktop shell (Windows).
#
# Produces:
#   - electron\metapi.exe                 — the Go server binary
#   - electron\dist\<platform>-<arch>\…   — the packaged desktop app
#
# Prerequisites: Go toolchain, Node.js >= 18, npm. If web\dist is missing,
# the script tries `make web-build` (which requires Bun); otherwise it errors.
[CmdletBinding()]
param(
  [switch]$SkipWebBuild
)

$ErrorActionPreference = 'Stop'
$Root = (git rev-parse --show-toplevel)
if ($LASTEXITCODE -ne 0) { $Root = (Get-Location).Path }
$ElectronDir = Join-Path $Root 'electron'
$GoOut = Join-Path $ElectronDir 'metapi.exe'

Write-Host "==> [1/4] ensure web\dist exists (Go embeds it)" -ForegroundColor Cyan
$webDist = Join-Path $Root 'web\dist'
if (-not (Test-Path -LiteralPath $webDist -PathType Container)) {
  if ($SkipWebBuild) {
    throw "web\dist is missing and -SkipWebBuild was set. Build the SPA first: cd web; bun install; bun run build:web"
  }
  $bun = Get-Command bun -ErrorAction SilentlyContinue
  if (-not $bun) {
    throw "web\dist is missing and bun is not installed. Build the SPA first: cd web; bun install; bun run build:web"
  }
  Write-Host "    web\dist missing - building frontend" -ForegroundColor DarkGray
  make web-build
  if ($LASTEXITCODE -ne 0) { throw "make web-build failed (exit $LASTEXITCODE)" }
} else {
  Write-Host "    web\dist present" -ForegroundColor DarkGray
}

Write-Host "==> [2/4] build the Go server binary -> electron\" -ForegroundColor Cyan
go build -trimpath -o $GoOut ./cmd/server
if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
Write-Host "    built: $GoOut" -ForegroundColor DarkGray

Write-Host "==> [3/4] install Electron dependencies" -ForegroundColor Cyan
Push-Location $ElectronDir
try {
  if (-not (Test-Path -LiteralPath (Join-Path $ElectronDir 'node_modules') -PathType Container)) {
    npm install
    if ($LASTEXITCODE -ne 0) { throw "npm install failed (exit $LASTEXITCODE)" }
  } else {
    Write-Host "    node_modules present, skipping npm install" -ForegroundColor DarkGray
  }

  Write-Host "==> [4/4] package the desktop app with electron-packager" -ForegroundColor Cyan
  npm run package
  if ($LASTEXITCODE -ne 0) { throw "npm run package failed (exit $LASTEXITCODE)" }
} finally {
  Pop-Location
}

Write-Host ""
Write-Host "Done. Packaged app is in electron\dist\." -ForegroundColor Green
