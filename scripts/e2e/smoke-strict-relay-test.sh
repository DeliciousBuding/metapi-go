#!/usr/bin/env bash
# Deterministic contract test for smoke.sh strict/skip relay semantics.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SMOKE_SCRIPT="${SMOKE_UNDER_TEST:-$ROOT_DIR/scripts/e2e/smoke.sh}"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

cat > "$TEST_DIR/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
set -euo pipefail
method=GET
out=
auth=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -X) method="$2"; shift 2 ;;
    -o) out="$2"; shift 2 ;;
    -w) shift 2 ;;
    -H)
      case "$2" in Authorization:*) auth="${2#Authorization: }" ;; esac
      shift 2
      ;;
    -d) shift 2 ;;
    -m) shift 2 ;;
    -s|-S|-f|-sS|-fsS) shift ;;
    http://*|https://*) url="$1"; shift ;;
    *) echo "fake curl: unsupported argument $1" >&2; exit 2 ;;
  esac
done
path="/${url#*://*/}"
status=200
body='{}'
case "$method $path" in
  "GET /health") body='{"status":"ok"}' ;;
  "GET /api/sites")
    if [ -z "$auth" ]; then
      status=401
      body='{"error":"unauthorized"}'
    else
      body='[{"id":1,"name":"e2e-smoke","platform":"new-api","url":"http://127.0.0.1:3001"}]'
    fi
    ;;
  "POST /api/sites/detect") body='{"platform":"new-api"}' ;;
  "POST /api/accounts/login") body='{"success":true,"account":{"accessToken":"session-token"}}' ;;
  "POST /api/accounts/verify-token") body='{"tokenType":"session"}' ;;
  "GET /api/accounts") body='{"accounts":[{"id":1,"siteId":1,"username":"root"}]}' ;;
  "GET /api/accounts/1/models")
    if [ "${STRICT_CASE:-good}" = "models_empty" ]; then
      body='{"models":[],"totalCount":0}'
    else
      body='{"models":[{"name":"gpt-4o-mini"}],"totalCount":1}'
    fi
    ;;
  "POST /api/accounts/1/balance") body='{"balance":10}' ;;
  "POST /api/checkin/trigger/1") body='{"success":true}' ;;
  "POST /api/downstream-keys") status=409; body='{"error":"duplicate"}' ;;
  "GET /api/downstream-keys") body='{"items":[{"id":9,"name":"e2e-smoke-token"}]}' ;;
  "PUT /api/downstream-keys/9") body='{"success":true}' ;;
  "GET /api/routes/lite") body='[{"id":1,"modelPattern":"gpt-4o-mini"},{"id":2,"modelPattern":"gpt-3.5-turbo"}]' ;;
  "GET /v1/models")
    if [ "${STRICT_CASE:-good}" = "models_empty" ]; then
      body='{"object":"list","data":[]}'
    else
      body='{"object":"list","data":[{"id":"gpt-4o-mini","object":"model"}]}'
    fi
    ;;
  "POST /v1/chat/completions")
    case "${STRICT_CASE:-good}" in
      completion_error) status=502; body='{"error":{"message":"no available channels"}}' ;;
      completion_missing) body='{"id":"chatcmpl-empty","choices":[{"message":{"role":"assistant"}}]}' ;;
      *) body='{"id":"chatcmpl-ok","choices":[{"message":{"role":"assistant","content":"metapi-e2e-marker"}}]}' ;;
    esac
    ;;
  *) status=404; body='{"error":"fake route not found"}' ;;
esac
printf '%s' "$body" > "$out"
printf '%s' "$status"
FAKE_CURL
chmod +x "$TEST_DIR/curl"

run_smoke() {
  local case_name="$1" output="$2"
  shift 2
  env PATH="$TEST_DIR:$PATH" STRICT_CASE="$case_name" \
    METAPI_URL=http://127.0.0.1:4000 \
    METAPI_AUTH_TOKEN=test-admin \
    UPSTREAM_URL=http://127.0.0.1:3001 \
    UPSTREAM_USERNAME=root \
    UPSTREAM_PASSWORD=test-password \
    PLATFORM=new-api \
    EXPECTED_COMPLETION_CONTENT=metapi-e2e-marker \
    "$@" bash "$SMOKE_SCRIPT" >"$output" 2>&1
}

assert_contains() {
  local file="$1" exact="$2"
  if ! grep -Fqx "$exact" "$file"; then
    echo "missing exact output: $exact" >&2
    cat "$file" >&2
    exit 1
  fi
}

good="$TEST_DIR/good.log"
if ! run_smoke good "$good"; then
  echo "strict happy path failed" >&2
  cat "$good" >&2
  exit 1
fi
echo "strict happy path: PASS"

if [ "${SKIP_EMPTY_MUTATION:-0}" != "1" ]; then
  after_empty="$TEST_DIR/models-empty.log"
  if run_smoke models_empty "$after_empty"; then
    echo "strict smoke accepted empty model lists" >&2
    cat "$after_empty" >&2
    exit 1
  fi
  assert_contains "$after_empty" '[FAIL] models (HTTP 200, totalCount=0, first model missing)'
  assert_contains "$after_empty" '[FAIL] proxy /v1/models (HTTP 200, expected non-empty data without error)'
  echo "empty-model mutation: rejected"
fi

completion_error="$TEST_DIR/completion-error.log"
if run_smoke completion_error "$completion_error"; then
  echo "strict smoke accepted a structured completion error" >&2
  cat "$completion_error" >&2
  exit 1
fi
assert_contains "$completion_error" '[FAIL] proxy /v1/chat/completions (HTTP 502, expected completion content without error)'
echo "structured-error mutation: rejected"

completion_missing="$TEST_DIR/completion-missing.log"
if run_smoke completion_missing "$completion_missing"; then
  echo "strict smoke accepted a completion without content" >&2
  cat "$completion_missing" >&2
  exit 1
fi
assert_contains "$completion_missing" '[FAIL] proxy /v1/chat/completions (HTTP 200, expected completion content without error)'
echo "missing-content mutation: rejected"

relaxed="$TEST_DIR/relaxed.log"
run_smoke models_empty "$relaxed" EXPECT_RELAY=0
assert_contains "$relaxed" '[SKIP] models relay assertion disabled explicitly (HTTP 200, totalCount=0)'
assert_contains "$relaxed" '[SKIP] proxy /v1/models relay assertion disabled explicitly'
assert_contains "$relaxed" '[SKIP] proxy /v1/chat/completions relay assertion disabled explicitly'
assert_contains "$relaxed" '== summary: 10 passed, 1 warned, 3 skipped, 0 failed =='
if grep -Fq '[PASS] models (' "$relaxed" || grep -Fq '[PASS] proxy /v1/' "$relaxed"; then
  echo "non-strict relay assertion was mislabeled PASS" >&2
  cat "$relaxed" >&2
  exit 1
fi
echo "explicit non-strict mode: 3 SKIP, 0 relay PASS"

assert_contains "$good" '[PASS] token reuse (e2e-smoke-token, relay policy reasserted)'
assert_contains "$good" '[PASS] proxy /v1/models (HTTP 200, non-empty data)'
assert_contains "$good" '[PASS] proxy /v1/chat/completions (HTTP 200, completion content present)'
assert_contains "$good" '== summary: 14 passed, 0 warned, 0 skipped, 0 failed =='
echo "smoke strict relay contract: PASS"
