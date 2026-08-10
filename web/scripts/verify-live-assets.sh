#!/usr/bin/env bash
# verify-live-assets.sh — post-deploy asset smoke (bash + curl, no node on target).
#
# Replays the browser asset graph against a live MetAPI instance:
#   1. index.html entry assets must answer 200 with a real content type
#   2. lazy chunks referenced by entry bundles must answer 200
#   3. any asset answered as text/html = SPA fallback swallowed it (fail)
#
# Usage: bash scripts/verify-live-assets.sh [baseUrl]   (default http://127.0.0.1:4000)
set -u

BASE="${1:-http://127.0.0.1:4000}"
BASE="${BASE%/}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail=0

fetch_ok() { # url -> 0 ok / 1 missing / 2 swallowed-by-spa
  local url="$1" ct
  local code
  code="$(curl -s -o "$TMP/body" -w '%{http_code}' "$url")"
  [ "$code" = "200" ] || { echo "  MISSING [$code] $url"; return 1; }
  ct="$(curl -sI "$url" | grep -i '^content-type:' | sed 's/.*: //' | tr -d '\r')"
  case "$ct" in
    text/html*) echo "  SWALLOWED $url answered text/html"; return 2 ;;
  esac
  return 0
}

# 1) entry assets from index.html
refs="$(curl -s "$BASE/" | grep -oE '(src|href)="(/assets/[^"]+)"' | sed -E 's/.*"(\/assets\/[^"]+)".*/\1/' | sort -u)"

# 2) lazy chunks from every JS bundle (extend the ref set, then check)
for rel in $refs; do
  case "$rel" in
    *.js)
      curl -s "$BASE$rel" -o "$TMP/$(basename "$rel")"
      newrefs="$(grep -oE 'import\(`\./[^`]+`\)' "$TMP/$(basename "$rel")" | sed -E 's/import\(`\.\/([^`]+)`\)/\/assets\/\1/' || true)"
      refs="$(printf '%s\n%s\n' "$refs" "$newrefs" | sort -u)"
      ;;
  esac
done

total=0
for rel in $refs; do
  total=$((total + 1))
  if ! fetch_ok "$BASE$rel"; then fail=1; fi
done

if [ "$fail" = 1 ]; then
  echo "verify-live-assets FAIL ($total referenced assets, errors above)"
  exit 1
fi
echo "verify-live-assets OK: $total referenced assets all 200 ($BASE)"
