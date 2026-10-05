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
  psql "$PG_TEST_DSN" -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA IF EXISTS $schema CASCADE" >/dev/null 2>&1
  if [[ $exit_code -ne 0 && -f "$tmp/server.log" ]]; then cat "$tmp/server.log" >&2; fi
  rm -rf "$tmp"
}
trap cleanup EXIT

psql "$PG_TEST_DSN" -v ON_ERROR_STOP=1 -c "CREATE SCHEMA $schema" >/dev/null
# pgx accepts search_path as a runtime connection parameter. Each run owns this
# schema; it never shares tables or migration state with test-pg's integration suite.
dsn="${PG_TEST_DSN}&search_path=${schema}"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
mock_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
start_server() {
  DB_TYPE=postgres DB_URL="$dsn" AUTH_TOKEN=pg-process-admin-token \
    PROXY_TOKEN=pg-process-proxy-token DATA_DIR="$tmp/data" PORT="$port" \
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
auth=(-H 'Authorization: Bearer pg-process-admin-token' -H 'Content-Type: application/json')
curl -fsS -X POST "$base/api/sites" "${auth[@]}" \
  -d "{\"name\":\"pg-process-e2e\",\"url\":\"http://127.0.0.1:$mock_port\",\"platform\":\"openai\"}" \
  >"$tmp/site.json"
site_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/site.json")"
curl -fsS -X POST "$base/api/accounts" "${auth[@]}" \
  -d "{\"siteId\":$site_id,\"username\":\"pg-e2e\",\"accessToken\":\"pg-process-upstream-key\",\"credentialMode\":\"apikey\",\"skipModelFetch\":true}" \
  >"$tmp/account.json"
account_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$tmp/account.json")"
curl -fsS -X POST "$base/api/accounts/$account_id/models/manual" "${auth[@]}" \
  -d '{"models":["gpt-4o-mini"]}' >/dev/null
curl -fsS -X POST "$base/api/routes" "${auth[@]}" \
  -d '{"modelPattern":"gpt-4o-mini","routeMode":"pattern","enabled":true}' >/dev/null
proxy_key="sk-pg-process-e2e-key" # leak-guard-allow:LG-2FC1FC1F (fixed, dummy E2E fixture key)
curl -fsS -X POST "$base/api/downstream-keys" "${auth[@]}" \
  -d "{\"name\":\"pg-process-e2e\",\"key\":\"$proxy_key\",\"supportedModels\":[\"*\"]}" >/dev/null

mock_log="$tmp/mock-requests.jsonl"
touch "$mock_log"
MOCK_OPENAI_HOST=127.0.0.1 MOCK_OPENAI_PORT="$mock_port" \
MOCK_OPENAI_MODELS=gpt-4o-mini MOCK_OPENAI_MARKER=pg-process-e2e-marker \
MOCK_OPENAI_LOG="$mock_log" python3 "$(cd "$(dirname "$0")" && pwd)/mock-openai.py" >"$tmp/mock.log" 2>&1 &
mock_pid=$!
mock_ready=
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$mock_port/health" >/dev/null 2>&1; then mock_ready=1; break; fi
  sleep 1
done
if [[ -z "$mock_ready" ]]; then cat "$tmp/mock.log" >&2; echo "mock upstream did not become ready" >&2; exit 1; fi
before="$(wc -l < "$mock_log" | tr -d ' ')"
curl -fsS "$base/v1/chat/completions" -H "Authorization: Bearer $proxy_key" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}' \
  >"$tmp/relay.json"
python3 - "$tmp/relay.json" "$mock_log" "$before" <<'PY'
import json, sys
response = json.load(open(sys.argv[1], encoding="utf-8"))
if response.get("choices", [{}])[0].get("message", {}).get("content") != "pg-process-e2e-marker":
    raise SystemExit("relay response did not contain the expected deterministic marker")
events = [json.loads(line) for line in open(sys.argv[2], encoding="utf-8") if line.strip()]
new_events = events[int(sys.argv[3]):]
if not new_events:
    raise SystemExit("mock upstream observed no new relayed request")
if not any(event.get("model") == "gpt-4o-mini" for event in new_events):
    raise SystemExit("new mock upstream request did not carry the expected model")
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
  python3 -c 'import json,sys; sites=json.load(sys.stdin); assert any(s.get("name")=="pg-process-e2e" for s in sites), "created site missing after PostgreSQL process restart"'
echo "PostgreSQL process E2E passed: production binary, PostgreSQL migration/readiness, admin setup, deterministic Chat relay, and restart persistence"
