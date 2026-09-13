#!/usr/bin/env bash
# Deterministic contract tests for relay strictness and check-in verdicts.
# These fixtures test the instruments; real-platform chains remain separate.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SMOKE_SCRIPT="${SMOKE_UNDER_TEST:-$ROOT_DIR/scripts/e2e/smoke.sh}"
TOKEN_IMPORT_SCRIPT="${TOKEN_IMPORT_UNDER_TEST:-$ROOT_DIR/scripts/e2e/verify-token-import.sh}"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

cat > "$TEST_DIR/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
set -euo pipefail
method=GET
out=
auth=
url=
request_body=
exit_code=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    -X) method="$2"; shift 2 ;;
    -o) out="$2"; shift 2 ;;
    -w) shift 2 ;;
    -H)
      case "$2" in Authorization:*) auth="${2#Authorization: }" ;; esac
      shift 2
      ;;
    -d) request_body="$2"; shift 2 ;;
    --noproxy) shift 2 ;;
    -m) shift 2 ;;
    -q|-s|-S|-f|-sS|-fsS) shift ;;
    http://*|https://*) url="$1"; shift ;;
    *) echo "fake curl: unsupported argument $1" >&2; exit 2 ;;
  esac
done
path="/${url#*://*/}"
printf "%s %s\n" "$method" "$path" >> "$RELAY_CALL_LOG"
status=200
body='{}'
exposed_model=gpt-4o-mini
if [ "${STRICT_CASE:-good}" = "aliased_route" ]; then
  exposed_model=customer-alias
elif [ -f "$RELAY_CALL_LOG.public-model" ]; then
  exposed_model="$(cat "$RELAY_CALL_LOG.public-model")"
fi
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
  "PUT /api/accounts/1") body='{"id":1}' ;;
  "GET /api/accounts/1/models")
    case "${STRICT_CASE:-good}" in
      models_empty|account_models_empty) body='{"models":[],"totalCount":0}' ;;
      models_http_error) status=503; body='{"error":"discovery unavailable"}' ;;
      models_malformed) body='{not-json' ;;
      models_bad_shape) body='{"models":[{"name":null}],"totalCount":1}' ;;
      models_zero_count) body='{"models":[{"name":"gpt-4o-mini"}],"totalCount":0}' ;;
      *) body='{"models":[{"name":"gpt-4o-mini"}],"totalCount":1}' ;;
    esac
    ;;
  "POST /api/accounts/1/balance") body='{"balance":10}' ;;
  "POST /api/checkin/trigger/1")
    case "${STRICT_CASE:-good}" in
      checkin_failed) body='{"success":false,"status":"failed","skipped":false,"message":"test upstream authentication failed"}' ;;
      checkin_malformed) body='{"message":"missing outcome fields"}' ;;
      checkin_contradictory) body='{"success":true,"status":"success","skipped":true}' ;;
      checkin_skipped) body='{"success":true,"status":"skipped","skipped":true}' ;;
      *) body='{"success":true,"status":"success","skipped":false}' ;;
    esac
    ;;
  "POST /api/downstream-keys") status=409; body='{"error":"duplicate"}' ;;
  "GET /api/downstream-keys") body='{"items":[{"id":9,"name":"e2e-smoke-token"}]}' ;;
  "PUT /api/downstream-keys/9") body='{"success":true}' ;;
  "GET /api/routes/lite")
    case "${STRICT_CASE:-good}" in
      fresh_route) body='[]' ;;
      aliased_route) body='[{"id":1,"modelPattern":"gpt-4o-mini","displayName":"customer-alias"}]' ;;
      *) body='[{"id":1,"modelPattern":"gpt-4o-mini"},{"id":2,"modelPattern":"gpt-3.5-turbo"}]' ;;
    esac
    ;;
  "POST /api/routes")
    body="$(python3 -c '
import json,sys
from pathlib import Path
route=json.loads(sys.argv[1])
public=(route.get("displayName") or route["modelPattern"]).strip()
Path(sys.argv[2]).write_text(public)
print(json.dumps(dict(route, id=1)))
' "$request_body" "$RELAY_CALL_LOG.public-model")"
    ;;
  "GET /v1/models")
    case "${STRICT_CASE:-good}" in
      models_empty|proxy_models_empty) body='{"object":"list","data":[]}' ;;
      proxy_models_missing_selected) body='{"data":[{"id":"some-other-model"}]}' ;;
      proxy_models_200_error) body='{"error":{},"data":[{"id":"gpt-4o-mini"}]}' ;;
      proxy_models_malformed) body='{not-json' ;;
      proxy_models_http_error) status=503; body='{"error":"no models available"}' ;;
      *) body="$(printf '{"object":"list","data":[{"id":"%s","object":"model"}]}' "$exposed_model")" ;;
    esac
    if [ "${STRICT_CASE:-good}" = "proxy_models_transport_error" ]; then exit_code=28; fi
    ;;
  "POST /v1/chat/completions")
    requested_model="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("model", ""))' "$request_body")"
    if [ "$requested_model" != "$exposed_model" ]; then
      echo "fake curl: completion did not use the advertised route model" >&2
      exit 2
    fi
    # Upstream-side evidence: the deterministic mock logs every request it
    # receives; the *_log_missing mutations deliberately drop the record.
    if [ "${STRICT_CASE:-good}" != "chat_log_missing" ] && [ -n "${MOCK_REQUEST_LOG:-}" ]; then
      printf '{"path":"/v1/chat/completions","model":"%s"}\n' "$requested_model" >> "$MOCK_REQUEST_LOG"
    fi
    case "${STRICT_CASE:-good}" in
      completion_error) status=502; body='{"error":{"message":"no available channels"}}' ;;
      completion_503_error) status=503; body='{"error":{"message":"no available channels"}}' ;;
      completion_200_error) body='{"error":{"message":"model failed"}}' ;;
      completion_error_with_content) body='{"error":{},"choices":[{"message":{"content":"metapi-e2e-marker"}}]}' ;;
      completion_empty_choices) body='{"choices":[]}' ;;
      completion_missing) body='{"id":"chatcmpl-empty","choices":[{"message":{"role":"assistant"}}]}' ;;
      completion_empty_content) body='{"choices":[{"message":{"content":"   "}}]}' ;;
      completion_malformed) body='{not-json' ;;
      completion_wrong_marker) body='{"choices":[{"message":{"content":"wrong-marker"}}]}' ;;
      completion_parts) body='{"choices":[{"message":{"content":[{"type":"text","text":"part content"}]}}]}' ;;
      *) body='{"id":"chatcmpl-ok","choices":[{"message":{"role":"assistant","content":"metapi-e2e-marker"}}]}' ;;
    esac
    if [ "${STRICT_CASE:-good}" = "completion_transport_error" ]; then exit_code=28; fi
    ;;
  "POST /v1/messages")
    requested_model="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("model", ""))' "$request_body")"
    if [ "$requested_model" != "$exposed_model" ]; then
      echo "fake curl: messages did not use the advertised route model" >&2
      exit 2
    fi
    if [ "${STRICT_CASE:-good}" != "messages_log_missing" ] && [ -n "${MOCK_REQUEST_LOG:-}" ]; then
      printf '{"path":"/v1/messages","model":"%s"}\n' "$requested_model" >> "$MOCK_REQUEST_LOG"
    fi
    case "${STRICT_CASE:-good}" in
      messages_wrong_model) body='{"id":"msg-1","type":"message","role":"assistant","model":"some-other-model","content":[{"type":"text","text":"metapi-e2e-marker"}]}' ;;
      messages_200_error) body='{"type":"error","error":{"type":"api_error","message":"boom"}}' ;;
      messages_empty_content) body="$(printf '{"id":"msg-1","type":"message","role":"assistant","model":"%s","content":[]}' "$requested_model")" ;;
      *) body="$(printf '{"id":"msg-1","type":"message","role":"assistant","model":"%s","content":[{"type":"text","text":"metapi-e2e-marker"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}' "$requested_model")" ;;
    esac
    ;;
  "POST /v1/responses")
    requested_model="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("model", ""))' "$request_body")"
    if [ "$requested_model" != "$exposed_model" ]; then
      echo "fake curl: responses did not use the advertised route model" >&2
      exit 2
    fi
    if [ "${STRICT_CASE:-good}" != "responses_log_missing" ] && [ -n "${MOCK_REQUEST_LOG:-}" ]; then
      printf '{"path":"/v1/responses","model":"%s"}\n' "$requested_model" >> "$MOCK_REQUEST_LOG"
    fi
    case "${STRICT_CASE:-good}" in
      responses_wrong_model) body='{"id":"resp-1","object":"response","model":"some-other-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"metapi-e2e-marker"}]}]}' ;;
      responses_200_error) body='{"object":"response","error":{"message":"boom"}}' ;;
      responses_empty_output) body="$(printf '{"id":"resp-1","object":"response","model":"%s","output":[]}' "$requested_model")" ;;
      *) body="$(printf '{"id":"resp-1","object":"response","model":"%s","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"metapi-e2e-marker"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}' "$requested_model")" ;;
    esac
    ;;
  *) status=404; body='{"error":"fake route not found"}' ;;
esac
printf '%s' "$body" > "$out"
printf '%s' "$status"
exit "$exit_code"
FAKE_CURL
chmod +x "$TEST_DIR/curl"

run_chain() {
  local script="$1" case_name="$2" output="$3"
  shift 3
  : > "$output.requests"
  : > "$output.mocklog"
  env PATH="$TEST_DIR:$PATH" STRICT_CASE="$case_name" RELAY_CALL_LOG="$output.requests" \
    MOCK_REQUEST_LOG="$output.mocklog" \
    EXPECT_RELAY=1 E2E_SKIP_RELAY=0 \
    METAPI_URL=http://127.0.0.1:4000 \
    METAPI_AUTH_TOKEN=test-admin \
    UPSTREAM_URL=http://127.0.0.1:3001 \
    UPSTREAM_USERNAME=root \
    UPSTREAM_PASSWORD=test-password \
    UPSTREAM_TOKEN=test-upstream-token \
    ACCOUNT_USERNAME=root SITE_NAME=e2e-smoke TOKEN_NAME=e2e-smoke-token \
    PROXY_MODEL=gpt-4o-mini PLATFORM=new-api \
    EXPECTED_COMPLETION_CONTENT=metapi-e2e-marker \
    "$@" bash "$script" >"$output" 2>&1
}

run_smoke() { run_chain "$SMOKE_SCRIPT" "$@"; }

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
assert_contains "$relaxed" '[SKIP] downstream token relay setup disabled explicitly'
assert_contains "$relaxed" '[SKIP] route relay setup disabled explicitly'
assert_contains "$relaxed" '[SKIP] proxy /v1/models relay assertion disabled explicitly'
assert_contains "$relaxed" '[SKIP] proxy /v1/chat/completions relay assertion disabled explicitly'
assert_contains "$relaxed" '[SKIP] proxy /v1/messages relay assertion disabled explicitly'
assert_contains "$relaxed" '[SKIP] proxy /v1/responses relay assertion disabled explicitly'
assert_contains "$relaxed" '== summary: 9 passed, 0 warned, 7 skipped, 0 failed =='
if grep -Fq '[PASS] models (' "$relaxed" || grep -Fq '[PASS] proxy /v1/' "$relaxed"; then
  echo "non-strict relay assertion was mislabeled PASS" >&2
  cat "$relaxed" >&2
  exit 1
fi
echo "explicit non-strict mode: 7 SKIP, 0 relay PASS"

assert_contains "$good" '[PASS] token reuse (e2e-smoke-token, relay policy reasserted)'
assert_contains "$good" '[PASS] proxy /v1/models (HTTP 200, non-empty data)'
assert_contains "$good" '[PASS] proxy /v1/chat/completions (HTTP 200, completion content present; upstream event recorded path=/v1/chat/completions model=gpt-4o-mini)'
assert_contains "$good" '[PASS] proxy /v1/messages (HTTP 200, message content present, model echoed; upstream event recorded path=/v1/messages model=gpt-4o-mini)'
assert_contains "$good" '[PASS] proxy /v1/responses (HTTP 200, response output present, model echoed; upstream event recorded path=/v1/responses model=gpt-4o-mini)'
assert_contains "$good" '== summary: 16 passed, 0 warned, 0 skipped, 0 failed =='
test "$(grep -Fxc 'POST /v1/chat/completions' "$good.requests")" = "1"
test "$(grep -Fxc 'POST /v1/messages' "$good.requests")" = "1"
test "$(grep -Fxc 'POST /v1/responses' "$good.requests")" = "1"
echo "three-protocol matrix: exact relay calls"
# Both entrypoints consume the same normalized API outcome, including the
# token-import path that used to have no SKIP counter at all.
for script in "$SMOKE_SCRIPT" "$TOKEN_IMPORT_SCRIPT"; do
  chain="$(basename "$script")"
  output="$TEST_DIR/$chain-checkin-good.log"
  if ! run_chain "$script" good "$output"; then
    echo "$chain: happy path failed" >&2
    cat "$output" >&2
    exit 1
  fi
  assert_contains "$output" '[PASS] checkin (success)'

  for case_name in checkin_failed checkin_malformed checkin_contradictory; do
    output="$TEST_DIR/$chain-$case_name.log"
    if run_chain "$script" "$case_name" "$output"; then
      echo "$chain: accepted $case_name" >&2
      cat "$output" >&2
      exit 1
    fi
    assert_contains "$output" '[FAIL] checkin (HTTP 200, expected a consistent success or skipped outcome)'
    echo "$chain: $case_name rejected"
  done

  output="$TEST_DIR/$chain-checkin-skipped.log"
  if ! run_chain "$script" checkin_skipped "$output"; then
    echo "$chain: an explicit skipped outcome incorrectly failed the chain" >&2
    cat "$output" >&2
    exit 1
  fi
  assert_contains "$output" '[SKIP] checkin (status=skipped; no successful check-in verified)'
  if grep -Fq '[PASS] checkin (' "$output" || ! grep -Eq '^== summary: .* 1 skipped, 0 failed ==$' "$output"; then
    echo "$chain: skipped check-in was mislabeled or not counted" >&2
    cat "$output" >&2
    exit 1
  fi
  echo "$chain: explicit check-in skip counted, not PASS"
done

# A failed discovery must not be papered over by a fallback/another route.
# Keep these before the token happy-path output assertions: the old verifier
# must be rejected because it ACCEPTS bad input, not because its prose differs.
assert_no_relay_requests() {
  local output="$1"
  test -f "$output.requests"
  if grep -Eq '^[A-Z]+ /(v1/|api/downstream-keys|api/routes)' "$output.requests"; then
    echo "unexpected relay/setup request: $output" >&2
    cat "$output.requests" >&2
    exit 1
  else
    test "$?" -eq 1 # grep read errors are not evidence of zero requests.
  fi
}

for case_name in models_empty account_models_empty models_http_error models_malformed models_bad_shape models_zero_count; do
  output="$TEST_DIR/token-$case_name.log"
  if run_chain "$TOKEN_IMPORT_SCRIPT" "$case_name" "$output"; then
    echo "token-import verifier accepted failed model discovery: $case_name" >&2
    cat "$output" >&2
    exit 1
  fi
  status=200
  if [ "$case_name" = "models_http_error" ]; then status=503; fi
  assert_contains "$output" "[FAIL] models (HTTP $status, expected non-empty discovery containing PROXY_MODEL when set)"
  assert_no_relay_requests "$output"
  echo "token-import $case_name: rejected before relay setup"
done

for case_name in proxy_models_empty proxy_models_missing_selected proxy_models_200_error proxy_models_malformed proxy_models_http_error proxy_models_transport_error; do
  output="$TEST_DIR/token-$case_name.log"
  if run_chain "$TOKEN_IMPORT_SCRIPT" "$case_name" "$output"; then
    echo "token-import verifier accepted an unavailable proxy model: $case_name" >&2
    cat "$output" >&2
    exit 1
  fi
  status=200
  if [ "$case_name" = "proxy_models_http_error" ]; then status=503; fi
  assert_contains "$output" "[FAIL] proxy /v1/models (HTTP $status, expected non-empty data containing selected model without error)"
  assert_contains "$output" '[FAIL] proxy /v1/chat/completions not attempted (selected model unavailable)'
  assert_contains "$output" '[FAIL] proxy /v1/messages not attempted (selected model unavailable)'
  assert_contains "$output" '[FAIL] proxy /v1/responses not attempted (selected model unavailable)'
  if grep -Eqx 'POST /v1/(chat/completions|messages|responses)' "$output.requests"; then
    echo "token-import verifier issued a model POST after failed model discovery" >&2
    exit 1
  else
    test "$?" -eq 1
  fi
  echo "token-import $case_name: rejected without model POST"
done

for case_name in completion_error completion_503_error completion_200_error completion_error_with_content completion_empty_choices completion_missing completion_empty_content completion_malformed completion_wrong_marker completion_transport_error; do
  output="$TEST_DIR/token-$case_name.log"
  if run_chain "$TOKEN_IMPORT_SCRIPT" "$case_name" "$output"; then
    echo "token-import verifier accepted an invalid completion: $case_name" >&2
    cat "$output" >&2
    exit 1
  fi
  status=200
  if [ "$case_name" = "completion_error" ]; then status=502; fi
  if [ "$case_name" = "completion_503_error" ]; then status=503; fi
  assert_contains "$output" "[FAIL] proxy /v1/chat/completions (HTTP $status, expected completion content without error)"
  test "$(grep -Fxc 'POST /v1/chat/completions' "$output.requests")" = "1"
  echo "token-import $case_name: rejected, no retry"
done

output="$TEST_DIR/token-good.log"
run_chain "$TOKEN_IMPORT_SCRIPT" good "$output" PROXY_MODEL=
assert_contains "$output" '[PASS] models (HTTP 200, selected=gpt-4o-mini)'
assert_contains "$output" '[PASS] proxy /v1/models (HTTP 200, selected model present: gpt-4o-mini)'
assert_contains "$output" '[PASS] proxy /v1/chat/completions (HTTP 200, completion content present; upstream event recorded path=/v1/chat/completions model=gpt-4o-mini)'
assert_contains "$output" '[PASS] proxy /v1/messages (HTTP 200, message content present, model echoed; upstream event recorded path=/v1/messages model=gpt-4o-mini)'
assert_contains "$output" '[PASS] proxy /v1/responses (HTTP 200, response output present, model echoed; upstream event recorded path=/v1/responses model=gpt-4o-mini)'
assert_contains "$output" '== summary: 15 passed, 0 warned, 0 skipped, 0 failed =='
test "$(grep -Fxc 'GET /v1/models' "$output.requests")" = "1"
test "$(grep -Fxc 'POST /v1/chat/completions' "$output.requests")" = "1"
test "$(grep -Fxc 'POST /v1/messages' "$output.requests")" = "1"
test "$(grep -Fxc 'POST /v1/responses' "$output.requests")" = "1"
echo "token-import default model selection: 15 PASS, exact relay calls"

output="$TEST_DIR/token-requested-model-missing.log"
if run_chain "$TOKEN_IMPORT_SCRIPT" good "$output" PROXY_MODEL=not-discovered; then
  echo "token-import verifier ignored the requested model" >&2
  exit 1
fi
assert_contains "$output" '[FAIL] models (HTTP 200, expected non-empty discovery containing PROXY_MODEL when set)'
assert_no_relay_requests "$output"

# Match smoke.sh: no marker means nonempty text (including text parts) suffices;
# a configured marker requires exact string equality, tested above.
output="$TEST_DIR/token-parts.log"
run_chain "$TOKEN_IMPORT_SCRIPT" completion_parts "$output" EXPECTED_COMPLETION_CONTENT=
assert_contains "$output" '== summary: 15 passed, 0 warned, 0 skipped, 0 failed =='

output="$TEST_DIR/token-explicit-skip.log"
run_chain "$TOKEN_IMPORT_SCRIPT" models_empty "$output" E2E_SKIP_RELAY=1
assert_contains "$output" '[SKIP] models relay assertion disabled explicitly'
assert_contains "$output" '[SKIP] downstream token relay setup disabled explicitly'
assert_contains "$output" '[SKIP] route relay setup disabled explicitly'
assert_contains "$output" '[SKIP] proxy /v1/models relay assertion disabled explicitly'
assert_contains "$output" '[SKIP] proxy /v1/chat/completions relay assertion disabled explicitly'
assert_contains "$output" '[SKIP] proxy /v1/messages relay assertion disabled explicitly'
assert_contains "$output" '[SKIP] proxy /v1/responses relay assertion disabled explicitly'
assert_contains "$output" '== summary: 8 passed, 0 warned, 7 skipped, 0 failed =='
assert_contains "$output" '[PASS] checkin (success)'
grep -Fqx 'GET /health' "$output.requests"
assert_no_relay_requests "$output"
if grep -Eq '^\[PASS\] (models|token|route|proxy) ' "$output"; then
  echo "explicit relay skip was mislabeled PASS" >&2
  exit 1
else
  test "$?" -eq 1
fi
echo "token-import explicit skip: 8 management PASS, 7 SKIP, zero relay/setup calls"

# Existing management checks still gate even when relay coverage is disabled.
output="$TEST_DIR/token-skip-checkin-failed.log"
if run_chain "$TOKEN_IMPORT_SCRIPT" checkin_failed "$output" E2E_SKIP_RELAY=1; then
  echo "relay skip masked a failed check-in" >&2
  exit 1
fi
assert_contains "$output" '[FAIL] checkin (HTTP 200, expected a consistent success or skipped outcome)'
assert_no_relay_requests "$output"

for flag in SKIP_MODEL_FETCH=true EXPECT_RELAY=0; do
  output="$TEST_DIR/token-not-a-skip-${flag%%=*}.log"
  if run_chain "$TOKEN_IMPORT_SCRIPT" models_empty "$output" "$flag"; then
    echo "token-import verifier treated $flag as relay skip" >&2
    exit 1
  fi
  assert_contains "$output" '[FAIL] models (HTTP 200, expected non-empty discovery containing PROXY_MODEL when set)'
  assert_no_relay_requests "$output"
done
output="$TEST_DIR/token-invalid-skip.log"
if run_chain "$TOKEN_IMPORT_SCRIPT" good "$output" E2E_SKIP_RELAY=true; then
  echo "token-import verifier accepted an ambiguous skip value" >&2
  exit 1
fi
assert_contains "$output" 'FATAL: E2E_SKIP_RELAY must be 0 or 1'
test ! -s "$output.requests"

# A fresh route exposes displayName as its model ID. The fixture must derive
# that value from the actual create payload instead of assuming the raw model.
for script in "$SMOKE_SCRIPT" "$TOKEN_IMPORT_SCRIPT"; do
  for case_name in fresh_route aliased_route; do
    output="$TEST_DIR/$(basename "$script")-$case_name.log"
    if ! run_chain "$script" "$case_name" "$output"; then
      echo "$(basename "$script"): $case_name did not relay the exposed model" >&2
      cat "$output" >&2
      exit 1
    fi
    if [ "$case_name" = fresh_route ]; then
      test "$(grep -Fxc 'POST /api/routes' "$output.requests")" = "1"
    elif grep -Fqx 'POST /api/routes' "$output.requests"; then
      echo "existing alias was overwritten instead of reused" >&2
      exit 1
    fi
    test "$(grep -Fxc 'POST /v1/chat/completions' "$output.requests")" = "1"
    test "$(grep -Fxc 'POST /v1/messages' "$output.requests")" = "1"
    test "$(grep -Fxc 'POST /v1/responses' "$output.requests")" = "1"
    echo "$(basename "$script"): $case_name relays the advertised model on all three protocols"
  done
done

# Without MOCK_REQUEST_LOG the client-side half of each protocol assertion
# still runs, and the upstream-evidence half reports explicit countable SKIPs.
# (CI wires the log path in the workflow; a bare run must degrade honestly.)
output="$TEST_DIR/smoke-no-mock-log.log"
run_smoke good "$output" MOCK_REQUEST_LOG=
assert_contains "$output" '[PASS] proxy /v1/chat/completions (HTTP 200, completion content present)'
assert_contains "$output" '[PASS] proxy /v1/messages (HTTP 200, message content present, model echoed)'
assert_contains "$output" '[PASS] proxy /v1/responses (HTTP 200, response output present, model echoed)'
assert_contains "$output" '[SKIP] proxy /v1/chat/completions upstream mock-log reconciliation (MOCK_REQUEST_LOG unset)'
assert_contains "$output" '[SKIP] proxy /v1/messages upstream mock-log reconciliation (MOCK_REQUEST_LOG unset)'
assert_contains "$output" '[SKIP] proxy /v1/responses upstream mock-log reconciliation (MOCK_REQUEST_LOG unset)'
assert_contains "$output" '== summary: 16 passed, 0 warned, 3 skipped, 0 failed =='
echo "smoke without MOCK_REQUEST_LOG: client-side PASS + 3 explicit reconciliation SKIPs"

output="$TEST_DIR/token-no-mock-log.log"
run_chain "$TOKEN_IMPORT_SCRIPT" good "$output" MOCK_REQUEST_LOG=
assert_contains "$output" '[PASS] proxy /v1/chat/completions (HTTP 200, completion content present)'
assert_contains "$output" '[SKIP] proxy /v1/messages upstream mock-log reconciliation (MOCK_REQUEST_LOG unset)'
assert_contains "$output" '[SKIP] proxy /v1/responses upstream mock-log reconciliation (MOCK_REQUEST_LOG unset)'
assert_contains "$output" '== summary: 15 passed, 0 warned, 3 skipped, 0 failed =='
echo "token-import without MOCK_REQUEST_LOG: client-side PASS + 3 explicit reconciliation SKIPs"

# --- protocol-matrix mutations: upstream-evidence reconciliation must go red ---
# Anti-vacuity proof for the mock-log half of every protocol assertion: when
# the deterministic upstream records no new event for the relayed model, the
# protocol FAILs even though the client-visible response was perfect, and the
# other two protocols still PASS on their own deltas.
for script in "$SMOKE_SCRIPT" "$TOKEN_IMPORT_SCRIPT"; do
  chain="$(basename "$script")"
  for case_name in chat_log_missing messages_log_missing responses_log_missing; do
    protocol="${case_name%%_log_missing}"
    path="/v1/chat/completions"
    [ "$protocol" = "messages" ] && path="/v1/messages"
    [ "$protocol" = "responses" ] && path="/v1/responses"
    output="$TEST_DIR/$chain-$case_name.log"
    if run_chain "$script" "$case_name" "$output"; then
      echo "$chain: accepted $case_name (no upstream-side event recorded)" >&2
      cat "$output" >&2
      exit 1
    fi
    assert_contains "$output" "[FAIL] proxy $path upstream evidence missing (HTTP 200, mock request log gained no new event for model gpt-4o-mini)"
    echo "$chain: $case_name rejected (client-green but upstream-silent)"
  done
done

# --- protocol-matrix mutations: echoed model, structured errors, empty content ---
for script in "$SMOKE_SCRIPT" "$TOKEN_IMPORT_SCRIPT"; do
  chain="$(basename "$script")"
  for case_name in messages_wrong_model messages_200_error messages_empty_content; do
    output="$TEST_DIR/$chain-$case_name.log"
    if run_chain "$script" "$case_name" "$output"; then
      echo "$chain: accepted $case_name" >&2
      cat "$output" >&2
      exit 1
    fi
    assert_contains "$output" '[FAIL] proxy /v1/messages (HTTP 200, expected message content with echoed model without error)'
    echo "$chain: $case_name rejected"
  done
  for case_name in responses_wrong_model responses_200_error responses_empty_output; do
    output="$TEST_DIR/$chain-$case_name.log"
    if run_chain "$script" "$case_name" "$output"; then
      echo "$chain: accepted $case_name" >&2
      cat "$output" >&2
      exit 1
    fi
    assert_contains "$output" '[FAIL] proxy /v1/responses (HTTP 200, expected response output with echoed model without error)'
    echo "$chain: $case_name rejected"
  done
done

echo "smoke/token-import strict relay and check-in verdict contracts: PASS"
