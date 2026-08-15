#!/usr/bin/env bash
# smoke.sh — idempotent real-platform e2e smoke chain for metapi-go.
#
# Exercises the full admin + proxy chain against a live metapi instance:
#   health -> admin auth -> platform detect -> site -> login -> verify-token
#   -> account -> models -> balance -> checkin -> downstream token -> route
#   -> /v1 proxy relay.
#
# Every step prints [PASS]/[FAIL]/[WARN] and accumulates a summary; the script
# exits 1 when any step FAILs. FAILs print the truncated response body as
# evidence. The script is re-runnable: sites/accounts/routes are looked up by
# name before creation, and the downstream key has a fixed value so a 409
# conflict is treated as "already exists".
#
# Configuration (environment variables):
#   METAPI_URL         admin base URL         (default http://127.0.0.1:4000)
#   METAPI_AUTH_TOKEN  admin Bearer token     (REQUIRED)
#   UPSTREAM_URL       upstream platform URL  (default http://127.0.0.1:3001)
#   UPSTREAM_USERNAME  upstream login user    (default root)
#   UPSTREAM_PASSWORD  upstream login pass    (default metapi123)
#   PLATFORM           expected platform      (default new-api)
#   PROXY_MODEL        fallback route model   (default gpt-3.5-turbo)
#   SITE_NAME          site name to reuse     (default e2e-smoke)
#   TOKEN_NAME         downstream key name    (default e2e-smoke-token)
#
# Requires curl and python3 (fails fast when missing).

set -uo pipefail

METAPI_URL="${METAPI_URL:-http://127.0.0.1:4000}"
METAPI_URL="${METAPI_URL%/}"
METAPI_AUTH_TOKEN="${METAPI_AUTH_TOKEN:-}"
UPSTREAM_URL="${UPSTREAM_URL:-http://127.0.0.1:3001}"
UPSTREAM_USERNAME="${UPSTREAM_USERNAME:-root}"
UPSTREAM_PASSWORD="${UPSTREAM_PASSWORD:-metapi123}"
PLATFORM="${PLATFORM:-new-api}"
PROXY_MODEL="${PROXY_MODEL:-gpt-3.5-turbo}"
SITE_NAME="${SITE_NAME:-e2e-smoke}"
TOKEN_NAME="${TOKEN_NAME:-e2e-smoke-token}"
SMOKE_KEY="${SMOKE_KEY:-sk-e2e-smoke-key}"

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

echo "== metapi e2e smoke =="
echo "   metapi=$METAPI_URL upstream=$UPSTREAM_URL platform=$PLATFORM"

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
    fail_step "detect (platform=$detected_platform, want $PLATFORM)"
    evidence
  fi
else
  fail_step "detect (HTTP $status)"
  evidence
fi

# 4. site idempotent-create
SITE_ID=""
status="$(request GET "$METAPI_URL/api/sites" "" "$METAPI_AUTH_TOKEN")"
if [ "$status" = "200" ]; then
  if idx="$(json_find_index name "$SITE_NAME" 2>/dev/null)"; then
    SITE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
  fi
fi
if [ -z "$SITE_ID" ]; then
  status="$(request POST "$METAPI_URL/api/sites" "{\"name\":\"$SITE_NAME\",\"url\":\"$UPSTREAM_URL\",\"platform\":\"$PLATFORM\"}" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ] || [ "$status" = "201" ]; then
    SITE_ID="$(json_value id 2>/dev/null || true)"
  elif [ "$status" = "409" ]; then
    # duplicate site exists (race between reads): re-read by name
    request GET "$METAPI_URL/api/sites" "" "$METAPI_AUTH_TOKEN" >/dev/null
    if idx="$(json_find_index name "$SITE_NAME" 2>/dev/null)"; then
      SITE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    fi
  fi
fi
if [ -n "$SITE_ID" ]; then
  pass_step "site idempotent-create (siteId=$SITE_ID)"
else
  fail_step "site idempotent-create (no site id obtained)"
  evidence
fi

# 5. login (creates/updates the account too)
LOGIN_TOKEN=""
if [ -n "$SITE_ID" ]; then
  status="$(request POST "$METAPI_URL/api/accounts/login" "{\"siteId\":$SITE_ID,\"username\":\"$UPSTREAM_USERNAME\",\"password\":\"$UPSTREAM_PASSWORD\"}" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    login_success="$(json_value success 2>/dev/null || true)"
    if [ "$login_success" = "True" ]; then
      LOGIN_TOKEN="$(json_value account.accessToken 2>/dev/null || true)"
      pass_step "login (siteId=$SITE_ID, token=${LOGIN_TOKEN:+present})"
    else
      fail_step "login (success=$login_success)"
      evidence
    fi
  else
    fail_step "login (HTTP $status)"
    evidence
  fi
else
  fail_step "login skipped (no site id)"
fi

# 6. verify-token — the token-paste path, exercised with the REAL session token
#    obtained from the login step (not the password).
if [ -n "$SITE_ID" ]; then
  status="$(request POST "$METAPI_URL/api/accounts/verify-token" "{\"siteId\":$SITE_ID,\"accessToken\":\"$LOGIN_TOKEN\",\"credentialMode\":\"session\"}" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    pass_step "verify-token (real session token, HTTP 200)"
  else
    fail_step "verify-token (real session token, HTTP $status)"
    evidence
  fi
else
  fail_step "verify-token skipped (no site id)"
fi

# 7. account idempotent-create
ACCOUNT_ID=""
if [ -n "$SITE_ID" ]; then
  status="$(request GET "$METAPI_URL/api/accounts" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    if idx="$(json_find_index2 siteId "$SITE_ID" username "$UPSTREAM_USERNAME" 2>/dev/null)"; then
      ACCOUNT_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    fi
  fi
  if [ -z "$ACCOUNT_ID" ]; then
    status="$(request POST "$METAPI_URL/api/accounts" "{\"siteId\":$SITE_ID,\"username\":\"$UPSTREAM_USERNAME\",\"accessToken\":\"$LOGIN_TOKEN\",\"credentialMode\":\"password\",\"checkinEnabled\":true}" "$METAPI_AUTH_TOKEN")"
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
  fi
  if [ -n "$ACCOUNT_ID" ]; then
    pass_step "account idempotent-create (accountId=$ACCOUNT_ID)"
  fi
else
  fail_step "account idempotent-create skipped (no site id)"
fi

# 8. models (frontend uses GET /api/accounts/{id}/models)
ROUTE_MODEL="$PROXY_MODEL"
if [ -n "$ACCOUNT_ID" ]; then
  status="$(request GET "$METAPI_URL/api/accounts/$ACCOUNT_ID/models" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    model_count="$(json_value totalCount 2>/dev/null || true)"
    first_model="$(json_value "models[0].name" 2>/dev/null || true)"
    if [ -n "$first_model" ]; then
      ROUTE_MODEL="$first_model"
    fi
    pass_step "models (HTTP 200, totalCount=$model_count)"
  else
    fail_step "models (HTTP $status)"
    evidence
  fi
else
  fail_step "models skipped (no account id)"
fi

# 9. balance (POST /api/accounts/{id}/balance)
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

# 10. checkin (POST /api/checkin/trigger/{id}); v1 may not support checkin —
#     a 2xx with success=false is a documented unsupported result (PASS),
#     5xx/crash is a FAIL.
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

# 11. downstream token create (POST /api/downstream-keys; fixed key => idempotent)
PROXY_TOKEN=""
status="$(request POST "$METAPI_URL/api/downstream-keys" "{\"name\":\"$TOKEN_NAME\",\"key\":\"$SMOKE_KEY\"}" "$METAPI_AUTH_TOKEN")"
if [ "$status" = "200" ] || [ "$status" = "201" ]; then
  PROXY_TOKEN="$SMOKE_KEY"
  pass_step "token create ($TOKEN_NAME)"
elif [ "$status" = "409" ]; then
  # duplicate key: same fixed value from a previous run — reuse it
  PROXY_TOKEN="$SMOKE_KEY"
  pass_step "token create (HTTP 409 duplicate, reusing fixed key)"
else
  fail_step "token create (HTTP $status)"
  evidence
fi

# 12. route create (idempotent by modelPattern lookup)
if [ -n "$PROXY_TOKEN" ]; then
  ROUTE_ID=""
  status="$(request GET "$METAPI_URL/api/routes/lite" "" "$METAPI_AUTH_TOKEN")"
  if [ "$status" = "200" ]; then
    if idx="$(json_find_index modelPattern "$ROUTE_MODEL" 2>/dev/null)"; then
      ROUTE_ID="$(json_value "[$idx].id" 2>/dev/null || true)"
    fi
  fi
  if [ -z "$ROUTE_ID" ]; then
    status="$(request POST "$METAPI_URL/api/routes" "{\"modelPattern\":\"$ROUTE_MODEL\",\"displayName\":\"e2e-smoke-route\",\"routeMode\":\"pattern\",\"enabled\":true}" "$METAPI_AUTH_TOKEN")"
    if [ "$status" = "200" ] || [ "$status" = "201" ]; then
      ROUTE_ID="$(json_value id 2>/dev/null || true)"
    fi
  fi
  if [ -n "$ROUTE_ID" ]; then
    if [ "$ROUTE_MODEL" = "$PROXY_MODEL" ] && [ -z "${first_model:-}" ]; then
      warn_step "route create (routeId=$ROUTE_ID, model=$ROUTE_MODEL) — upstream model list empty, used PROXY_MODEL"
    else
      pass_step "route create (routeId=$ROUTE_ID, model=$ROUTE_MODEL)"
    fi
  else
    fail_step "route create (no route id obtained)"
    evidence
  fi
else
  fail_step "route create skipped (no downstream token)"
fi

# 13. proxy relay
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
    # error (e.g. upstream has no channels). Internal 5xx + non-JSON would fail.
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
