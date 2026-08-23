#!/usr/bin/env bash
# verify-live-assets.sh — post-deploy asset smoke (bash + curl, no node on target).
#
# Replays the browser asset graph against a live Metapi instance:
#   1. index.html entry assets must answer 200 with a real content type
#   2. lazy chunks listed in the runtime chunk map must answer 200
#   3. any asset answered as text/html = SPA fallback swallowed it (fail)
#   4. zero discovered assets = fail-closed (an empty set proves nothing)
#
# Entry assets match both the rsbuild output prefix (/static/...) and the
# legacy Vite prefix (/assets/...). Lazy chunks are discovered from the
# rspack runtime chunk-id->content-hash map inside each entry bundle
# (c.u=e=>"static/js/async/"+e+"."+({ID:"HASH",...})[e]+".js"); minified
# rsbuild output contains no import() literals, so parsing bundle text for
# them (the old approach) can never find anything.
#
# Usage: bash scripts/verify-live-assets.sh [baseUrl]   (default http://127.0.0.1:4000)
#
# All curl calls use --noproxy '*': this is a localhost smoke test, and a
# host-level proxy (~/.curlrc, http_proxy) would falsify the result.
set -u

BASE="${1:-http://127.0.0.1:4000}"
BASE="${BASE%/}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail=0

fetch_ok() { # url -> 0 ok / 1 missing / 2 swallowed-by-spa
  local url="$1" ct
  local code
  code="$(curl --noproxy '*' -s -o "$TMP/body" -w '%{http_code}' "$url")"
  [ "$code" = "200" ] || { echo "  MISSING [$code] $url"; return 1; }
  ct="$(curl --noproxy '*' -sI "$url" | grep -i '^content-type:' | sed 's/.*: //' | tr -d '\r')"
  case "$ct" in
    text/html*) echo "  SWALLOWED $url answered text/html"; return 2 ;;
  esac
  return 0
}

# 1) entry assets from index.html (rsbuild emits /static/, Vite emitted /assets/)
refs="$(curl --noproxy '*' -s "$BASE/" | grep -oE '(src|href)="(/(static|assets)/[^"]+)"' | sed -E 's/.*"(\/(static|assets)\/[^"]+)".*/\1/' | sort -u)"

# 2) lazy chunks from every entry JS bundle (extend the ref set, then check)
for rel in $refs; do
  case "$rel" in
    *.js)
      curl --noproxy '*' -s "$BASE$rel" -o "$TMP/$(basename "$rel")"
      newrefs="$(grep -oE '"static/js/async/"[^{]*\{[^}]+\}' "$TMP/$(basename "$rel")" \
        | grep -oE '[0-9]+:"[0-9a-f]+"' \
        | sed -E 's|([0-9]+):"([0-9a-f]+)"|/static/js/async/\1.\2.js|' || true)"
      refs="$(printf '%s\n%s\n' "$refs" "$newrefs" | grep -v '^$' | sort -u)"
      ;;
  esac
done

total=0
for rel in $refs; do
  total=$((total + 1))
  if ! fetch_ok "$BASE$rel"; then fail=1; fi
done

# Fail-closed: an empty ref set means nothing was verified (wrong BASE,
# index.html unreachable, or the asset prefix changed again). Never report
# OK on an empty set.
if [ "$total" -eq 0 ]; then
  echo "verify-live-assets FAIL: no /static/ or /assets/ entry assets discovered at $BASE/ (nothing verified)"
  exit 1
fi

if [ "$fail" = 1 ]; then
  echo "verify-live-assets FAIL ($total referenced assets, errors above)"
  exit 1
fi
echo "verify-live-assets OK: $total referenced assets all 200 ($BASE)"
