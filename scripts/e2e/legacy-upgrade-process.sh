#!/usr/bin/env bash
# Boot the production server binary on a copy of the legacy TS SQLite fixture.
# This covers process startup + real HTTP reads after the in-place upgrade.
set -euo pipefail

if [[ $# -ne 1 || ! -x "$1" ]]; then
  echo "usage: $0 <metapi-server-binary>" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
pid=
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

mkdir -p "$tmp/data"
cp "$root/store/testdata/ts-source/hub.db" "$tmp/data/hub.db"
python3 - "$tmp/data/hub.db" <<'PY'
import sqlite3
import sys

with sqlite3.connect(sys.argv[1]) as db:
    columns = {row[1] for row in db.execute("PRAGMA table_info(sites)")}
missing = {"max_concurrency", "custom_headers_override_request_headers"} & columns
assert not missing, f"fixture already has additive sites columns {sorted(missing)}; refresh the legacy-field assertion"
PY
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
AUTH_TOKEN=legacy-upgrade-admin-token \
PROXY_TOKEN=legacy-upgrade-proxy-token \
DATA_DIR="$tmp/data" \
PORT="$port" \
"$1" >"$tmp/server.log" 2>&1 &
pid=$!

ready=
for _ in $(seq 1 40); do
  if ! kill -0 "$pid" 2>/dev/null; then
    cat "$tmp/server.log" >&2
    echo "server exited before readiness" >&2
    exit 1
  fi
  if curl -fsS "http://127.0.0.1:$port/ready" 2>/dev/null |
      python3 -c 'import json,sys; r=json.load(sys.stdin); assert r == {"status":"ok","database":"ok"}' 2>/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [[ -z "$ready" ]]; then
  cat "$tmp/server.log" >&2
  echo "legacy database server did not become ready" >&2
  exit 1
fi

kill -0 "$pid"
curl -fsS -H 'Authorization: Bearer legacy-upgrade-admin-token' \
  "http://127.0.0.1:$port/api/sites" |
  python3 -c '
import json, sys
sites = json.load(sys.stdin)
assert len(sites) == 10, f"expected 10 legacy sites, got {len(sites)}"
site = next((s for s in sites if s.get("url") == "https://zhongzhuan-a.example.invalid"), None)
assert site is not None, "legacy site missing after startup upgrade"
max_concurrency = site.get("maxConcurrency")
assert max_concurrency == 0, f"maxConcurrency={max_concurrency!r}, want additive default 0"
assert site.get("customHeadersOverrideRequestHeaders") is False, "customHeadersOverrideRequestHeaders did not receive its additive default"'
kill -0 "$pid"
echo "legacy fixture process upgrade passed: readiness, data, and additive fields verified"
