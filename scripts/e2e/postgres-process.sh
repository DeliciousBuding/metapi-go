#!/usr/bin/env bash
# Exercise the production server process against an isolated PostgreSQL schema.
set -euo pipefail

if [[ $# -ne 1 || ! -x "$1" ]]; then
  echo "usage: $0 <metapi-server-binary>" >&2
  exit 2
fi
: "${PG_TEST_DSN:?PG_TEST_DSN must point at the disposable CI PostgreSQL service}"

tmp="$(mktemp -d)"
schema="metapi_e2e_$(python3 -c 'import secrets; print(secrets.token_hex(6))')"
pid=
mock_pid=
sibling_pid=
cleanup() {
  exit_code=$?
  if [[ -n "$pid" ]]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$mock_pid" ]]; then
    kill "$mock_pid" >/dev/null 2>&1 || true
    wait "$mock_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$sibling_pid" ]]; then
    kill "$sibling_pid" >/dev/null 2>&1 || true
    wait "$sibling_pid" >/dev/null 2>&1 || true
  fi
  psql "$PG_TEST_DSN" -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA IF EXISTS $schema CASCADE" >/dev/null 2>&1
  if [[ $exit_code -ne 0 && -f "$tmp/server.log" ]]; then tail -n 50 "$tmp/server.log" >&2; fi
  rm -rf "$tmp"
}
trap cleanup EXIT

psql "$PG_TEST_DSN" -v ON_ERROR_STOP=1 -c "CREATE SCHEMA $schema" >/dev/null
# pgx accepts search_path as a runtime connection parameter. Each run owns this
# schema; it never shares tables or migration state with test-pg's integration suite.
dsn="${PG_TEST_DSN}&search_path=${schema}"
# libpq rejects the pgx URL's search_path parameter. Use PGOPTIONS for psql
# while the server receives the schema-scoped pgx URL.
psql_schema() { PGOPTIONS="-c search_path=$schema" psql "$PG_TEST_DSN" -v ON_ERROR_STOP=1 "$@"; }
psql_schema -q -f "$(dirname "$0")/fixtures/postgres-legacy-sites.sql"
pre_upgrade_columns="$(psql_schema -Atqc "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'sites' AND column_name IN ('max_concurrency', 'custom_headers_override_request_headers')")"
if [[ "$pre_upgrade_columns" != 0 ]]; then
  echo "legacy PostgreSQL fixture no longer exercises the additive sites upgrade" >&2
  exit 1
fi
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
mock_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
sibling_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
start_server() {
  DB_TYPE=postgres DB_URL="$dsn" AUTH_TOKEN=pg-process-admin-token \
    PROXY_TOKEN=pg-process-proxy-token DATA_DIR="$tmp/data" PORT="$port" \
    PROXY_MAX_CHANNEL_ATTEMPTS=2 PROXY_LOG_ASYNC=false \
    DISABLE_CROSS_PROTOCOL_FALLBACK=true \
    "$1" >"$tmp/server.log" 2>&1 &
  pid=$!
}
wait_ready() {
  for _ in $(seq 1 60); do
    if ! kill -0 "$pid" 2>/dev/null; then
      cat "$tmp/server.log" >&2
      echo "PostgreSQL server exited before readiness" >&2
      return 1
    fi
    if curl -fsS "http://127.0.0.1:$port/ready" 2>/dev/null |
      python3 -c 'import json,sys; assert json.load(sys.stdin) == {"status":"ok","database":"ok"}' 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  cat "$tmp/server.log" >&2
  echo "PostgreSQL server did not become ready" >&2
  return 1
}

start_server "$1"
wait_ready
base="http://127.0.0.1:$port"
curl -fsS "$base/api/sites" -H 'Authorization: Bearer pg-process-admin-token' |
  python3 -c 'import json,sys; sites=json.load(sys.stdin); old=[s for s in sites if s.get("id")==41 and s.get("name")=="pg-legacy-site"]; assert len(old)==1, "legacy PostgreSQL row missing from admin HTTP read"; assert old[0].get("maxConcurrency")==0, "additive integer default missing from admin HTTP read"; assert old[0].get("customHeadersOverrideRequestHeaders") is False, "additive boolean default missing from admin HTTP read"'
psql_schema -Atqc "SELECT id, created_at, max_concurrency, custom_headers_override_request_headers FROM sites WHERE id = 41" |
  python3 -c 'import sys; assert sys.stdin.read().strip()=="41|2026-01-01T12:34:56Z|0|f", "legacy PostgreSQL row or additive defaults not persisted"'
psql_schema -Atqc "SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'sites' AND column_name = 'custom_headers_override_request_headers'" |
  python3 -c 'import sys; assert sys.stdin.read().strip()=="boolean", "additive PostgreSQL boolean column has the wrong type"'
psql_schema -Atqc "SELECT COUNT(*) FROM schema_migrations WHERE version IN ('sc2_002_site_max_concurrency', 'sc2_008_site_custom_headers_override_request_headers')" |
  python3 -c 'import sys; assert sys.stdin.read().strip()=="2", "legacy PostgreSQL additive migrations were not recorded"'
auth=(-H 'Authorization: Bearer pg-process-admin-token' -H 'Content-Type: application/json')
curl -fsS -X POST "$base/api/sites" "${auth[@]}" \
  -d "{\"name\":\"pg-process-e2e\",\"url\":\"http://127.0.0.1:$mock_port\",\"platform\":\"openai\"}" \
  >"$tmp/site.json"
site_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/site.json")"
if (( site_id <= 41 )); then
  echo "PostgreSQL site sequence did not advance beyond the legacy row" >&2
  exit 1
fi
curl -fsS -X POST "$base/api/accounts" "${auth[@]}" \
  -d "{\"siteId\":$site_id,\"username\":\"pg-e2e\",\"accessToken\":\"pg-process-upstream-key\",\"credentialMode\":\"apikey\",\"skipModelFetch\":true}" \
  >"$tmp/account.json"
account_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/account.json")"
curl -fsS -X POST "$base/api/accounts/$account_id/models/manual" "${auth[@]}" \
  -d '{"models":["gpt-4o-mini"]}' >/dev/null
curl -fsS -X POST "$base/api/sites" "${auth[@]}" \
  -d "{\"name\":\"pg-process-sibling\",\"url\":\"http://127.0.0.1:$sibling_port\",\"platform\":\"openai\"}" \
  >"$tmp/sibling-site.json"
sibling_site_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/sibling-site.json")"
curl -fsS -X POST "$base/api/accounts" "${auth[@]}" \
  -d "{\"siteId\":$sibling_site_id,\"username\":\"pg-e2e-sibling\",\"accessToken\":\"pg-process-sibling-key\",\"credentialMode\":\"apikey\",\"skipModelFetch\":true}" \
  >"$tmp/sibling-account.json"
sibling_account_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/sibling-account.json")"
curl -fsS -X POST "$base/api/accounts/$sibling_account_id/models/manual" "${auth[@]}" \
  -d '{"models":["gpt-4o-mini"]}' >/dev/null
curl -fsS -X POST "$base/api/routes" "${auth[@]}" \
  -d '{"modelPattern":"gpt-4o-mini","routeMode":"pattern","enabled":true}' >"$tmp/route.json"
route_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/route.json")"
curl -fsS "$base/api/routes/$route_id/channels" "${auth[@]}" >"$tmp/channels.json"
read -r bad_channel sibling_channel < <(python3 - "$tmp/channels.json" "$account_id" "$sibling_account_id" <<'PY'
import json, sys
channels = json.load(open(sys.argv[1], encoding="utf-8"))
accounts = [int(sys.argv[2]), int(sys.argv[3])]
assert len(channels) == 2, f"expected exactly two PostgreSQL route channels, got {len(channels)}"
by_account = {channel["accountId"]: channel["id"] for channel in channels}
assert len(by_account) == 2 and set(by_account) == set(accounts), "route channels do not match both upstream accounts"
print(by_account[accounts[0]], by_account[accounts[1]])
PY
)
if [[ -z "$bad_channel" || -z "$sibling_channel" ]]; then
  echo "could not identify both PostgreSQL route channels" >&2
  exit 1
fi
curl -fsS -X PUT "$base/api/channels/$bad_channel" "${auth[@]}" -d '{"priority":0}' >/dev/null
curl -fsS -X PUT "$base/api/channels/$sibling_channel" "${auth[@]}" -d '{"priority":1}' >/dev/null
proxy_key="sk-pg-process-e2e-key" # leak-guard-allow:LG-2FC1FC1F (fixed, dummy E2E fixture key)
curl -fsS -X POST "$base/api/downstream-keys" "${auth[@]}" \
  -d "{\"name\":\"pg-process-e2e\",\"key\":\"$proxy_key\",\"supportedModels\":[\"*\"]}" >/dev/null

mock_log="$tmp/mock-requests.jsonl"
sibling_log="$tmp/sibling-requests.jsonl"
touch "$mock_log"
touch "$sibling_log"
MOCK_OPENAI_HOST=127.0.0.1 MOCK_OPENAI_PORT="$mock_port" \
MOCK_OPENAI_MODELS=gpt-4o-mini MOCK_OPENAI_MARKER=pg-process-primary-marker \
MOCK_OPENAI_LOG="$mock_log" python3 "$(cd "$(dirname "$0")" && pwd)/mock-openai.py" >"$tmp/mock.log" 2>&1 &
mock_pid=$!
MOCK_OPENAI_HOST=127.0.0.1 MOCK_OPENAI_PORT="$sibling_port" \
MOCK_OPENAI_MODELS=gpt-4o-mini MOCK_OPENAI_MARKER=pg-process-sibling-marker \
MOCK_OPENAI_LOG="$sibling_log" python3 "$(cd "$(dirname "$0")" && pwd)/mock-openai.py" >"$tmp/sibling.log" 2>&1 &
sibling_pid=$!
mock_ready=
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$mock_port/health" >/dev/null 2>&1; then mock_ready=1; break; fi
  sleep 1
done
if [[ -z "$mock_ready" ]]; then cat "$tmp/mock.log" >&2; echo "mock upstream did not become ready" >&2; exit 1; fi
mock_ready=
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$sibling_port/health" >/dev/null 2>&1; then mock_ready=1; break; fi
  sleep 1
done
if [[ -z "$mock_ready" ]]; then cat "$tmp/sibling.log" >&2; echo "sibling mock upstream did not become ready" >&2; exit 1; fi
before="$(wc -l < "$mock_log" | tr -d ' ')"
curl -fsS "$base/v1/chat/completions" -H "Authorization: Bearer $proxy_key" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}' \
  >"$tmp/relay.json"
python3 - "$tmp/relay.json" "$mock_log" "$before" <<'PY'
import json, sys
response = json.load(open(sys.argv[1], encoding="utf-8"))
if response.get("choices", [{}])[0].get("message", {}).get("content") != "pg-process-primary-marker":
    raise SystemExit("healthy priority-0 upstream was not selected before cascade")
events = [json.loads(line) for line in open(sys.argv[2], encoding="utf-8") if line.strip()]
new_events = events[int(sys.argv[3]):]
if not new_events:
    raise SystemExit("mock upstream observed no new relayed request")
if not any(event.get("model") == "gpt-4o-mini" for event in new_events):
    raise SystemExit("new mock upstream request did not carry the expected model")
PY

# The first request established which channel wins. Shut it down, then require
# the same downstream route to recover through the lower-priority sibling.
kill "$mock_pid"
wait "$mock_pid" || true
mock_pid=
if curl -fsS "http://127.0.0.1:$mock_port/health" >/dev/null 2>&1; then
  echo "primary mock remained live after process stop" >&2
  exit 1
fi
sibling_before="$(wc -l < "$sibling_log" | tr -d ' ')"
request_id="pg-cascade-${schema}"
curl -fsS "$base/v1/chat/completions" -H "Authorization: Bearer $proxy_key" \
  -H 'Content-Type: application/json' -H "X-Request-Id: $request_id" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"cascade"}]}' \
  -D "$tmp/cascade.headers" >"$tmp/cascade.json"
psql_schema -Atqc "SELECT channel_id, status, http_status, retry_count FROM proxy_logs WHERE request_id = '$request_id' ORDER BY retry_count, id" >"$tmp/cascade.rows"
python3 - "$tmp/cascade.json" "$tmp/cascade.headers" "$sibling_log" "$sibling_before" "$tmp/cascade.rows" "$bad_channel" "$sibling_channel" "$request_id" <<'PY'
import json, sys
response = json.load(open(sys.argv[1], encoding="utf-8"))
assert response.get("choices", [{}])[0].get("message", {}).get("content") == "pg-process-sibling-marker", "cascade did not return the sibling marker"
headers = open(sys.argv[2], encoding="utf-8").read().lower().splitlines()
assert f"x-request-id: {sys.argv[8]}" in headers, "cascade response lost its request ID"
events = [json.loads(line) for line in open(sys.argv[3], encoding="utf-8") if line.strip()]
assert events[int(sys.argv[4]):] == [{"path": "/v1/chat/completions", "model": "gpt-4o-mini"}], "sibling mock did not observe exactly one relayed request"
rows = [line.strip() for line in open(sys.argv[5], encoding="utf-8") if line.strip()]
want = [f"{sys.argv[6]}|failed|502|0", f"{sys.argv[7]}|success|200|1"]
assert rows == want, f"cascade PostgreSQL proxy logs = {rows!r}, want {want!r}"
PY

curl -fsS "$base/api/sites" -H 'Authorization: Bearer pg-process-admin-token' |
  python3 -c 'import json,sys; assert any(s.get("name")=="pg-process-e2e" for s in json.load(sys.stdin)), "created site missing before restart"'
kill "$pid"
wait "$pid" || true
pid=
if curl -fsS "http://127.0.0.1:$port/ready" >/dev/null 2>&1; then
  echo "server port remained live after process stop" >&2
  exit 1
fi
start_server "$1"
wait_ready
curl -fsS -H 'Authorization: Bearer pg-process-admin-token' "http://127.0.0.1:$port/api/sites" |
  python3 -c 'import json,sys; sites=json.load(sys.stdin); assert any(s.get("name")=="pg-process-e2e" for s in sites), "created site missing after PostgreSQL process restart"; assert any(s.get("id")==41 and s.get("name")=="pg-legacy-site" for s in sites), "legacy PostgreSQL site missing after restart"'
psql_schema -Atqc "SELECT COUNT(*) FROM schema_migrations WHERE version IN ('sc2_002_site_max_concurrency', 'sc2_008_site_custom_headers_override_request_headers')" |
  python3 -c 'import sys; assert sys.stdin.read().strip()=="2", "additive migration journal changed after PostgreSQL restart"'
echo "PostgreSQL process E2E passed: legacy sites upgrade, production binary, two-mock channel cascade, and restart persistence"
