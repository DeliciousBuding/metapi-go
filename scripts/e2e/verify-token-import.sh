#!/usr/bin/env bash
# verify-token-import.sh — idempotent e2e chain for TOKEN-IMPORT platforms.
#
# Companion to smoke.sh (the password-login chain). This script drives the
# token-paste flow for platforms that authenticate via an imported credential
# (Sub2API session JWT, CLIProxyAPI management key, etc.) instead of a
# username/password login:
#   health -> admin auth -> platform detect -> site -> verify-token
#   -> account -> models -> balance -> checkin -> downstream token -> route
#   -> /v1 proxy relay.
#
# Every step prints [PASS]/[FAIL]/[WARN] and accumulates a summary; the script
# exits 1 when any step FAILs. FAILs print the truncated response body as
# evidence.
#
# Re-runnable means CONVERGE, not skip (#1209). Every resource is looked up
# before it is created, and when it already exists this script re-asserts the
# state the current run needs instead of trusting whatever the previous run left
# stored:
#   site            reuse by name, then by URL (identity IS the name/URL)
#   account         re-bind the credential this run holds and just verified
#   downstream key  re-assert supportedModels ["*"] and enabled
#   route           reuse by modelPattern
# Each step prints which of the three it did -- created, reused or refreshed --
# so a green run cannot hide a resource that was merely left alone. A short-lived
# upstream credential is the case that made this necessary: skipping left the
# expired value stored and the failure surfaced two steps later on balance,
# looking like a product regression while a release candidate was being proved.
#
# Configuration (environment variables):
#   METAPI_URL         admin base URL         (default http://127.0.0.1:4000)
#   METAPI_AUTH_TOKEN  admin Bearer token     (REQUIRED)
#   UPSTREAM_URL       upstream platform URL  (REQUIRED)
#   UPSTREAM_TOKEN     upstream credential    (REQUIRED; session JWT / API key
#                                              / management key — no password)
#   PLATFORM           expected platform      (default sub2api)
#   CREDENTIAL_MODE    account mode to bind   (default session;
#                                              use apikey for e.g. cliproxyapi)
#   ACCOUNT_USERNAME   account username       (default e2e-token-import)
#   SKIP_MODEL_FETCH   "true" skips the upstream model verify on account
#                      create (needed when the upstream exposes no models,
#                      e.g. a fresh CLIProxyAPI instance with no auth files)
#   PROXY_MODEL        fallback route model   (default gpt-3.5-turbo)
#   SITE_NAME          site name to reuse     (default e2e-token-import)
#   TOKEN_NAME         downstream key name    (default e2e-token-import-token)
#
# Requires curl and python3 (fails fast when missing).

set -uo pipefail

METAPI_URL="${METAPI_URL:-http://127.0.0.1:4000}"
METAPI_URL="${METAPI_URL%/}"
METAPI_AUTH_TOKEN="${METAPI_AUTH_TOKEN:-}"
UPSTREAM_URL="${UPSTREAM_URL:-}"
UPSTREAM_TOKEN="${UPSTREAM_TOKEN:-}"
PLATFORM="${PLATFORM:-sub2api}"
CREDENTIAL_MODE="${CREDENTIAL_MODE:-session}"
ACCOUNT_USERNAME="${ACCOUNT_USERNAME:-e2e-token-import}"
SKIP_MODEL_FETCH="${SKIP_MODEL_FETCH:-false}"
PROXY_MODEL="${PROXY_MODEL:-gpt-3.5-turbo}"
SITE_NAME="${SITE_NAME:-e2e-token-import}"
TOKEN_NAME="${TOKEN_NAME:-e2e-token-import-token}"
TOKEN_IMPORT_KEY="${TOKEN_IMPORT_KEY:-sk-e2e-token}"

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0
FAILED_NAMES=""

# --- fatal setup checks ---

command -v curl >/dev/null 2>&1 || { echo "FATAL: curl is required but not found on PATH" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "FATAL: python3 is required for JSON parsing but not found on PATH" >&2; exit 2; }
if [ -z "$METAPI_AUTH_TOKEN" ]; then
  echo "FATAL: METAPI_AUTH_TOKEN is required (admin Bearer token)" >&2
  exit 2
fi
if [ -z "$UPSTREAM_URL" ]; then
  echo "FATAL: UPSTREAM_URL is required (upstream platform URL)" >&2
  exit 2
fi
if [ -z "$UPSTREAM_TOKEN" ]; then
  echo "FATAL: UPSTREAM_TOKEN is required (token-import credential)" >&2
  exit 2
fi

# Force UTF-8 for python stdin/stdout pipes (Windows python defaults to the
# locale codepage and mangles UTF-8 JSON bodies).
export PYTHONUTF8=1
export PYTHONIOENCODING=utf-8

WORKDIR="$(mktemp -d)" || { echo "FATAL: cannot create temp dir" >&2; exit 2; }
trap 'rm -rf "$WORKDIR"' EXIT
RESP_BODY="$WORKDIR/resp_body.txt"

# --- helpers ---

# request METHOD URL [JSON_BODY] [AUTH_TOKEN] -> prints HTTP status code;
# response body is saved to $RESP_BODY.
request() {
  local method="$1" url="$2" body="${3:-}" token="${4:-}"
  local args=(-sS -m 120 -X "$method" "$url" -o "$RESP_BODY" -w "%{http_code}" -H "Content-Type: application/json")
  if [ -n "$token" ]; then
    args+=(-H "Authorization: Bearer $token")
  fi
  if [ -n "$body" ]; then
    args+=(-d "$body")
  fi
  curl "${args[@]}" 2>"$WORKDIR/curl_err.txt"
}

body() { cat "$RESP_BODY" 2>/dev/null || true; }

# json_value EXPR — evaluates a dotted path against $RESP_BODY (JSON document).
# EXPR segments: field names, with optional [i] list indexing, e.g. "account.accessToken".
# Uses python3 -c (no heredoc): heredocs + stdin redirects misbehave under
# Git Bash for Windows, where the file redirect can override the heredoc.
json_value() {
  python3 -c '
import json, sys
expr = sys.argv[1]
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(3)
# The accounts snapshot is wrapped as {"accounts":[...]}; when the path starts
# with a list index, apply it to the inner list.
if expr.startswith("[") and isinstance(data, dict) and isinstance(data.get("accounts"), list):
    data = data["accounts"]
for part in expr.split("."):
    if part.startswith("[") and part.endswith("]"):
        try:
            data = data[int(part[1:-1])]
        except Exception:
            sys.exit(4)
    elif isinstance(data, dict) and part in data:
        data = data[part]
    else:
        sys.exit(4)
print("" if data is None else data)
' "$1" < "$RESP_BODY"
}

# json_find_index KEY VALUE — first list index i where item[KEY]==VALUE in $RESP_BODY.
json_find_index() {
  python3 -c '
import json, sys
key, want = sys.argv[1], sys.argv[2]
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(3)
if not isinstance(data, list):
    sys.exit(4)
for i, item in enumerate(data):
    if isinstance(item, dict) and str(item.get(key)) == want:
        print(i)
        sys.exit(0)
sys.exit(4)
' "$1" "$2" < "$RESP_BODY"
}

# json_find_index2 KEY1 VALUE1 KEY2 VALUE2 — first list index matching both pairs.
# Handles both a bare JSON array and the accounts wrapper {"accounts":[...]}.
json_find_index2() {
  python3 -c '
import json, sys
k1, v1, k2, v2 = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(3)
if isinstance(data, dict) and isinstance(data.get("accounts"), list):
    data = data["accounts"]
if not isinstance(data, list):
    sys.exit(4)
for i, item in enumerate(data):
    if isinstance(item, dict) and str(item.get(k1)) == v1 and str(item.get(k2)) == v2:
        print(i)
        sys.exit(0)
sys.exit(4)
' "$1" "$2" "$3" "$4" < "$RESP_BODY"
}

# json_has_error_key — 0 when $RESP_BODY is valid JSON containing an "error" key.
json_has_error_key() {
  python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(1)
if isinstance(data, dict) and "error" in data:
    sys.exit(0)
sys.exit(1)
' < "$RESP_BODY"
}

pass_step() { PASS_COUNT=$((PASS_COUNT + 1)); echo "[PASS] $1"; }
fail_step() { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_NAMES="$FAILED_NAMES $1"; echo "[FAIL] $1"; }
warn_step() { WARN_COUNT=$((WARN_COUNT + 1)); echo "[WARN] $1"; }

evidence() {
  local b curl_err
  b="$(body)"
  if [ -n "$b" ]; then
    if [ "${#b}" -gt 300 ]; then
      printf '       body: %.300s...\n' "$b"
    else
      printf '       body: %s\n' "$b"
    fi
  fi
  curl_err="$(cat "$WORKDIR/curl_err.txt" 2>/dev/null || true)"
  if [ -n "$curl_err" ]; then
    printf '       curl: %s\n' "$curl_err"
  fi
}

# --- main chain ---

echo "== metapi token-import e2e =="
echo "   metapi=$METAPI_URL upstream=$UPSTREAM_URL platform=$PLATFORM mode=$CREDENTIAL_MODE skipModelFetch=$SKIP_MODEL_FETCH"

# 1. health
status="$(request GET "$METAPI_URL/health")"
if [ "$status" = "200" ]; then
  pass_step "health (HTTP 200)"
else
  fail_step "health (HTTP $status)"
  evidence
fi

# 2. admin auth enforced
status_no="$(request GET "$METAPI_URL/api/sites" "" "")"
status_with="$(request GET "$METAPI_URL/api/sites" "" "$METAPI_AUTH_TOKEN")"
if [ "$status_no" = "401" ] && [ "$status_with" = "200" ]; then
  pass_step "admin auth enforced (no token -> $status_no, with token -> $status_with)"
else
  fail_step "admin auth enforced (no token -> $status_no, with token -> $status_with)"
  evidence
fi

# 3. platform detect
status="$(request POST "$METAPI_URL/api/sites/detect" "{\"url\":\"$UPSTREAM_URL\"}" "$METAPI_AUTH_TOKEN")"
if [ "$status" = "200" ]; then
  detected_platform="$(json_value platform 2>/dev/null || true)"
  if [ "$detected_platform" = "$PLATFORM" ]; then
    pass_step "detect (platform=$detected_platform)"
  else
    warn_step "detect (platform=$detected_platform, want $PLATFORM — creating site with explicit platform)"
    evidence
  fi
else
  fail_step "detect (HTTP $status)"
  evidence
fi

# 4. site idempotent-create (platform explicit so a flaky detect cannot block)
SITE_ID=""
SITE_STATE="created"
status="$(request GET "$METAPI_URL/api/sites" "" "$METAPI_AUTH_TOKEN")"
if [ "$status" = "200" ]; then
  if idx="$(json_find_index name "$SITE_NAME" 2>/dev/null)"; then
    SITE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    SITE_STATE="reused"
  fi
fi
if [ -z "$SITE_ID" ]; then
  status="$(request POST "$METAPI_URL/api/sites" "{\"name\":\"$SITE_NAME\",\"url\":\"$UPSTREAM_URL\",\"platform\":\"$PLATFORM\"}" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ] || [ "$status" = "201" ]; then
    SITE_ID="$(json_value id 2>/dev/null || true)"
  fi
  if [ -z "$SITE_ID" ]; then
    # Create failed (e.g. 409 duplicate): sites are unique per URL+platform,
    # so a previous run may have registered the same URL under a different
    # name. Re-read and match by name first, then by URL.
    request GET "$METAPI_URL/api/sites" "" "$METAPI_AUTH_TOKEN" >/dev/null
    if idx="$(json_find_index name "$SITE_NAME" 2>/dev/null)"; then
      SITE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    fi
    if [ -z "$SITE_ID" ]; then
      if idx="$(json_find_index url "$UPSTREAM_URL" 2>/dev/null)"; then
        SITE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
        SITE_STATE="reused-by-url"
        warn_step "site idempotent-create (reused siteId=$SITE_ID by URL match)"
      fi
    fi
  fi
fi
if [ -n "$SITE_ID" ]; then
  pass_step "site idempotent-create (siteId=$SITE_ID, $SITE_STATE)"
else
  fail_step "site idempotent-create (no site id obtained)"
  evidence
fi

# 5. verify-token — the token-paste path with the real imported credential
VERIFIED_TYPE=""
if [ -n "$SITE_ID" ]; then
  status="$(request POST "$METAPI_URL/api/accounts/verify-token" "{\"siteId\":$SITE_ID,\"accessToken\":\"$UPSTREAM_TOKEN\",\"credentialMode\":\"$CREDENTIAL_MODE\"}" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    VERIFIED_TYPE="$(json_value tokenType 2>/dev/null || true)"
    pass_step "verify-token (tokenType=$VERIFIED_TYPE)"
  else
    fail_step "verify-token (HTTP $status)"
    evidence
  fi
else
  fail_step "verify-token skipped (no site id)"
fi

# 6. account idempotent-create -- converge the credential, do not skip (#1209)
ACCOUNT_ID=""
ACCOUNT_STATE="created"
if [ -n "$SITE_ID" ] && [ -n "$VERIFIED_TYPE" ]; then
  status="$(request GET "$METAPI_URL/api/accounts" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    if idx="$(json_find_index2 siteId "$SITE_ID" username "$ACCOUNT_USERNAME" 2>/dev/null)"; then
      ACCOUNT_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    fi
  fi
  if [ -z "$ACCOUNT_ID" ]; then
    status="$(request POST "$METAPI_URL/api/accounts" "{\"siteId\":$SITE_ID,\"username\":\"$ACCOUNT_USERNAME\",\"accessToken\":\"$UPSTREAM_TOKEN\",\"credentialMode\":\"$CREDENTIAL_MODE\",\"checkinEnabled\":false,\"skipModelFetch\":$SKIP_MODEL_FETCH}" "$METAPI_AUTH_TOKEN")"
    if [ "$status" = "200" ]; then
      ACCOUNT_ID="$(json_value id 2>/dev/null || true)"
    elif [ "$status" = "400" ]; then
      req_verification="$(json_value requiresVerification 2>/dev/null || true)"
      if [ "$req_verification" = "True" ]; then
        fail_step "account create requiresVerification (body recorded)"
        evidence
      else
        fail_step "account create (HTTP 400)"
        evidence
      fi
    else
      fail_step "account create (HTTP $status)"
      evidence
    fi
  else
    # The account already exists, so it still carries whatever credential the
    # previous run stored. On a platform whose credential is short-lived -- a
    # sub2api session JWT -- that value is expired by now: verify-token passes
    # because it uses the fresh token from the environment, while balance fails
    # because it refreshes the STORED one, and the run dies two steps away from
    # the thing it is actually testing (#1209). Re-bind the credential this run
    # already holds and verified in step 5.
    #
    # Deliberately the same token and never a second issuance: sub2api bumps
    # token_version per issuance, so signing another one would invalidate the
    # token the rest of this run still needs. The refresh is printed rather than
    # silent, so it cannot mask a real "account lost its credential" regression.
    status="$(request PUT "$METAPI_URL/api/accounts/$ACCOUNT_ID" "{\"accessToken\":\"$UPSTREAM_TOKEN\",\"credentialMode\":\"$CREDENTIAL_MODE\"}" "$METAPI_AUTH_TOKEN")"
    if [ "$status" = "200" ]; then
      ACCOUNT_STATE="refreshed"
    else
      fail_step "account credential refresh (HTTP $status)"
      evidence
    fi
  fi
  if [ -n "$ACCOUNT_ID" ]; then
    pass_step "account idempotent-create (accountId=$ACCOUNT_ID, tokenType=$VERIFIED_TYPE, $ACCOUNT_STATE)"
  fi
else
  fail_step "account idempotent-create skipped (no site id / token not verified)"
fi

# 7. models (frontend uses GET /api/accounts/{id}/models)
ROUTE_MODEL="$PROXY_MODEL"
first_model=""
if [ -n "$ACCOUNT_ID" ]; then
  status="$(request GET "$METAPI_URL/api/accounts/$ACCOUNT_ID/models" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    model_count="$(json_value totalCount 2>/dev/null || true)"
    first_model="$(json_value "models[0].name" 2>/dev/null || true)"
    if [ -n "$first_model" ]; then
      ROUTE_MODEL="$first_model"
    fi
    if [ "$model_count" = "0" ] || [ -z "$model_count" ]; then
      warn_step "models (HTTP 200, totalCount=$model_count — upstream model list empty, route falls back to PROXY_MODEL)"
    else
      pass_step "models (HTTP 200, totalCount=$model_count)"
    fi
  else
    fail_step "models (HTTP $status)"
    evidence
  fi
else
  fail_step "models skipped (no account id)"
fi

# 8. balance (POST /api/accounts/{id}/balance)
if [ -n "$ACCOUNT_ID" ]; then
  status="$(request POST "$METAPI_URL/api/accounts/$ACCOUNT_ID/balance" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    pass_step "balance (HTTP 200)"
  else
    fail_step "balance (HTTP $status)"
    evidence
  fi
else
  fail_step "balance skipped (no account id)"
fi

# 9. checkin (POST /api/checkin/trigger/{id}); token-import platforms may not
#    support checkin — a 2xx with success=false is a documented unsupported
#    result (PASS), 5xx/crash is a FAIL.
if [ -n "$ACCOUNT_ID" ]; then
  status="$(request POST "$METAPI_URL/api/checkin/trigger/$ACCOUNT_ID" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    checkin_ok="$(json_value success 2>/dev/null || true)"
    if [ "$checkin_ok" = "True" ]; then
      pass_step "checkin (success)"
    else
      pass_step "checkin (documented unsupported/negative result: success=$checkin_ok)"
    fi
  elif [ "$status" = "404" ]; then
    fail_step "checkin (HTTP 404 account not found)"
    evidence
  else
    fail_step "checkin (HTTP $status)"
    evidence
  fi
else
  fail_step "checkin skipped (no account id)"
fi

# 10. downstream token create (POST /api/downstream-keys; fixed key => idempotent).
#     Managed keys default to deny-all-when-empty, so supportedModels:["*"]
#     explicitly lets the test model through to the router/upstream.
PROXY_TOKEN=""
status="$(request POST "$METAPI_URL/api/downstream-keys" "{\"name\":\"$TOKEN_NAME\",\"key\":\"$TOKEN_IMPORT_KEY\",\"supportedModels\":[\"*\"]}" "$METAPI_AUTH_TOKEN")"
if [ "$status" = "200" ] || [ "$status" = "201" ]; then
  PROXY_TOKEN="$TOKEN_IMPORT_KEY"
  pass_step "token create ($TOKEN_NAME)"
elif [ "$status" = "409" ]; then
  # The key exists from a previous run. Reassert the relay policy instead of
  # merely reusing it: an empty supportedModels list is intentionally deny-all,
  # so a leftover key would silently block every model this run is about to
  # relay. smoke.sh step 11 already converges this way (#1209).
  status="$(request GET "$METAPI_URL/api/downstream-keys" "" "$METAPI_AUTH_TOKEN")"
  key_id="$(python3 -c 'import json,sys; p=json.load(sys.stdin); name=sys.argv[1]; print(next(str(item.get("id", "")) for item in p.get("items", []) if isinstance(item, dict) and item.get("name") == name))' "$TOKEN_NAME" < "$RESP_BODY" 2>/dev/null || true)"
  if [ "$status" = "200" ] && [ -n "$key_id" ]; then
    status="$(request PUT "$METAPI_URL/api/downstream-keys/$key_id" '{"supportedModels":["*"],"enabled":true}' "$METAPI_AUTH_TOKEN")"
    if [ "$status" = "200" ]; then
      PROXY_TOKEN="$TOKEN_IMPORT_KEY"
      pass_step "token reuse ($TOKEN_NAME, relay policy reasserted)"
    else
      fail_step "token reuse/update (HTTP $status, keyId=$key_id)"
      evidence
    fi
  else
    # A 409 with no record under this name means the fixed key VALUE belongs to a
    # differently named record, so this script does not own it and cannot
    # reassert its policy. That is a real topology here: two chains share
    # TOKEN_IMPORT_KEY under different TOKEN_NAMEs, so whichever runs second
    # always lands in this branch. Reusing the value is still correct -- the
    # relay steps below prove it works -- but claiming the policy was checked
    # would be a green that means nothing, and failing would red a run whose
    # relay is fine. So: reuse, and say out loud what was not verified.
    PROXY_TOKEN="$TOKEN_IMPORT_KEY"
    warn_step "token reuse (HTTP 409: the fixed key is owned by a differently named record, relay policy NOT reasserted)"
  fi
else
  fail_step "token create (HTTP $status)"
  evidence
fi

# 11. route create (idempotent by modelPattern lookup)
if [ -n "$PROXY_TOKEN" ]; then
  ROUTE_ID=""
  ROUTE_STATE="created"
  status="$(request GET "$METAPI_URL/api/routes/lite" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    if idx="$(json_find_index modelPattern "$ROUTE_MODEL" 2>/dev/null)"; then
      ROUTE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
      ROUTE_STATE="reused"
    fi
  fi
  if [ -z "$ROUTE_ID" ]; then
    status="$(request POST "$METAPI_URL/api/routes" "{\"modelPattern\":\"$ROUTE_MODEL\",\"displayName\":\"e2e-token-import-route\",\"routeMode\":\"pattern\",\"enabled\":true}" "$METAPI_AUTH_TOKEN")"
    if [ "$status" = "200" ] || [ "$status" = "201" ]; then
      ROUTE_ID="$(json_value id 2>/dev/null || true)"
    fi
  fi
  if [ -n "$ROUTE_ID" ]; then
    if [ "$ROUTE_MODEL" = "$PROXY_MODEL" ] && [ -z "$first_model" ]; then
      warn_step "route create (routeId=$ROUTE_ID, model=$ROUTE_MODEL, $ROUTE_STATE) — upstream model list empty, used PROXY_MODEL"
    else
      pass_step "route create (routeId=$ROUTE_ID, model=$ROUTE_MODEL, $ROUTE_STATE)"
    fi
  else
    fail_step "route create (no route id obtained)"
    evidence
  fi
else
  fail_step "route create skipped (no downstream token)"
fi

# 12. proxy relay
if [ -n "$PROXY_TOKEN" ]; then
  status="$(request GET "$METAPI_URL/v1/models" "" "$PROXY_TOKEN")"
  if [ "$status" = "200" ]; then
    pass_step "proxy /v1/models (HTTP 200)"
  else
    fail_step "proxy /v1/models (HTTP $status)"
    evidence
  fi

  status="$(request POST "$METAPI_URL/v1/chat/completions" "{\"model\":\"$ROUTE_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}" "$PROXY_TOKEN")"
  if [ "$status" = "200" ]; then
    pass_step "proxy /v1/chat/completions (HTTP 200, relayed)"
  elif json_has_error_key; then
    # Structured JSON error: metapi relayed and answered with a documented
    # error (e.g. upstream has no models/channels). Internal 5xx + non-JSON
    # would fail.
    pass_step "proxy /v1/chat/completions (HTTP $status, structured error relayed)"
  else
    fail_step "proxy /v1/chat/completions (HTTP $status, unstructured response)"
    evidence
  fi
else
  fail_step "proxy skipped (no downstream token)"
fi

# --- summary ---
echo "== summary: $PASS_COUNT passed, $WARN_COUNT warned, $FAIL_COUNT failed =="
if [ "$FAIL_COUNT" -gt 0 ]; then
  echo "   failed steps:$FAILED_NAMES"
  exit 1
fi
exit 0
