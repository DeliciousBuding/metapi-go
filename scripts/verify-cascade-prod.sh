#!/usr/bin/env bash
# verify-cascade-prod.sh — P0-585 production multi-channel cascade verification.
#
# Connects to a running metapi instance (any reachable host — set METAPI_URL)
# and captures honest, READ-ONLY evidence of the multi-channel cascade
# failover behaviour.
# This script performs NO destructive actions: it does not disable channels,
# rotate credentials, or mutate routing state. It only:
#   1. checks health / readiness
#   2. snapshots the configured topology (sites / routes / channels)
#   3. records Prometheus counters before a test request
#   4. sends ONE minimal chat-completions request (if a model is available)
#   5. records Prometheus counters after, and diffs them
#   6. pulls recent proxy_logs and groups them by request_id to surface any
#      multi-channel retry (cascade) attempts already recorded by the instance
#
# Cascade evidence comes from rows in proxy_logs that share a request_id but
# differ on channel_id / retry_count — that is the same on-disk truth the unit
# and HTTP e2e tests assert against, observed against a live instance.
#
# ---------------------------------------------------------------------------
# Usage:
#   # On the production host itself (instance bound to 127.0.0.1:4000):
#   ssh <prod-host> 'bash -s' < scripts/verify-cascade-prod.sh
#
#   # Locally against an SSH tunnel to the production host:
#   ssh -L 4000:127.0.0.1:4000 <prod-host> -N   # in one terminal
#   METAPI_URL=http://127.0.0.1:4000 \
#   METAPI_AUTH_TOKEN=<admin-token> \
#   METAPI_PROXY_TOKEN=<proxy-token> \
#   ./scripts/verify-cascade-prod.sh
#
#   # Override the model used for the live probe (recommended):
#   METAPI_TEST_MODEL=gpt-4o-mini ./scripts/verify-cascade-prod.sh
#
# Required env:
#   METAPI_AUTH_TOKEN  — admin AUTH_TOKEN (for /api/* topology + proxy_logs)
#   METAPI_PROXY_TOKEN — downstream PROXY_TOKEN (for /v1/* live probe)
#
# Optional env:
#   METAPI_URL          default http://127.0.0.1:4000
#   METAPI_TEST_MODEL   model for the live probe (auto-detected if unset)
#   METAPI_REQUEST_TIMEOUT  default 30 (seconds)
#   METAPI_REPORT_DIR   default ./cascade-verify-reports (JSON report output)
# ---------------------------------------------------------------------------
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
METAPI_URL="${METAPI_URL:-http://127.0.0.1:4000}"
METAPI_AUTH_TOKEN="${METAPI_AUTH_TOKEN:-}"
METAPI_PROXY_TOKEN="${METAPI_PROXY_TOKEN:-}"
METAPI_TEST_MODEL="${METAPI_TEST_MODEL:-}"
METAPI_REQUEST_TIMEOUT="${METAPI_REQUEST_TIMEOUT:-30}"
METAPI_REPORT_DIR="${METAPI_REPORT_DIR:-./cascade-verify-reports}"

# Strip a trailing slash so URL composition is predictable.
METAPI_URL="${METAPI_URL%/}"

TIMESTAMP_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TIMESTAMP_FILE="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$METAPI_REPORT_DIR"
REPORT_JSON="$METAPI_REPORT_DIR/cascade-verify-${TIMESTAMP_FILE}.json"
RAW_DIR="$METAPI_REPORT_DIR/raw-${TIMESTAMP_FILE}"
mkdir -p "$RAW_DIR"

# Color helpers (disabled when not a TTY so log scraping stays clean).
if [ -t 1 ]; then
	C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
	C_GREEN=""; C_RED=""; C_YELLOW=""; C_DIM=""; C_RESET=""
fi

log()      { printf '%s[p0585]%s %s\n' "$C_DIM" "$C_RESET" "$*"; }
log_ok()   { printf '%s[p0585] OK   %s%s\n'  "$C_GREEN" "$C_RESET" "$*"; }
log_warn() { printf '%s[p0585] WARN %s%s\n'  "$C_YELLOW" "$C_RESET" "$*"; }
log_err()  { printf '%s[p0585] ERR  %s%s\n'  "$C_RED" "$C_RESET" "$*" >&2; }

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
preflight_failures=0
require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		log_err "required command not found: $1"
		preflight_failures=$((preflight_failures + 1))
	fi
}
require_cmd curl
require_cmd jq

if [ -z "$METAPI_AUTH_TOKEN" ]; then
	log_err "METAPI_AUTH_TOKEN is empty — admin /api/* checks will fail"
	preflight_failures=$((preflight_failures + 1))
fi
if [ -z "$METAPI_PROXY_TOKEN" ]; then
	log_err "METAPI_PROXY_TOKEN is empty — live /v1/* probe will fail"
	preflight_failures=$((preflight_failures + 1))
fi
if [ "$preflight_failures" -gt 0 ]; then
	log_err "preflight failed ($preflight_failures). Set METAPI_AUTH_TOKEN and METAPI_PROXY_TOKEN."
	exit 2
fi

# Initialise the JSON report skeleton. Fields are filled in as the script runs.
report_init() {
	jq -n \
		--arg timestamp "$TIMESTAMP_ISO" \
		--arg url "$METAPI_URL" \
		'{
			feature: "P0-585 multi-channel cascade failover",
			timestamp: $timestamp,
			instanceUrl: $url,
			verdict: "partial",
			verified: [],
			residual: [],
			topology: {},
			metricsDiff: {},
			liveProbe: {},
			cascadeEvidence: {}
		}' > "$REPORT_JSON"
}
report_set() { jq --argjson v "$2" "$1 = $v" "$REPORT_JSON" > "$REPORT_JSON.tmp" && mv "$REPORT_JSON.tmp" "$REPORT_JSON"; }
report_push_verified() { jq --arg v "$1" '.verified += [$v]' "$REPORT_JSON" > "$REPORT_JSON.tmp" && mv "$REPORT_JSON.tmp" "$REPORT_JSON"; }
report_push_residual() { jq --arg v "$1" '.residual += [$v]' "$REPORT_JSON" > "$REPORT_JSON.tmp" && mv "$REPORT_JSON.tmp" "$REPORT_JSON"; }
report_save_raw() { cp "$1" "$RAW_DIR/$2" 2>/dev/null || true; }

report_init

log "verifying P0-585 cascade against $METAPI_URL (report: $REPORT_JSON)"

# ---------------------------------------------------------------------------
# HTTP helpers (curl with auth + timeout, capturing status + body)
# ---------------------------------------------------------------------------
curl_public() {
	# $1 = path
	curl -sS -o "$RAW_DIR/$2" -w '%{http_code}' \
		--max-time "$METAPI_REQUEST_TIMEOUT" \
		"${METAPI_URL}${1}" 2>"$RAW_DIR/$2.err" || true
}
curl_admin() {
	# $1 = path, $2 = raw filename
	curl -sS -o "$RAW_DIR/$2" -w '%{http_code}' \
		-H "Authorization: Bearer ${METAPI_AUTH_TOKEN}" \
		--max-time "$METAPI_REQUEST_TIMEOUT" \
		"${METAPI_URL}${1}" 2>"$RAW_DIR/$2.err" || true
}
curl_proxy_json() {
	# $1 = path, $2 = raw filename, $3 = JSON body
	curl -sS -D "$RAW_DIR/$2.headers" -o "$RAW_DIR/$2.body" -w '%{http_code}' \
		-H "Authorization: Bearer ${METAPI_PROXY_TOKEN}" \
		-H "Content-Type: application/json" \
		-H "X-Metapi-Verify: cascade-p0585" \
		-X POST --data "$3" \
		--max-time "$METAPI_REQUEST_TIMEOUT" \
		"${METAPI_URL}${1}" 2>"$RAW_DIR/$2.err" || true
}

# ---------------------------------------------------------------------------
# 1. Health & readiness
# ---------------------------------------------------------------------------
log "step 1: health / readiness"
health_code="$(curl_public /health health.json)"
ready_code="$(curl_public /ready ready.json)"
report_set '.health' "$(jq -n \
	--argjson healthStatus "$health_code" \
	--argjson readyStatus "$ready_code" \
	'{health: $healthStatus, ready: $readyStatus}')"

if [ "$health_code" = "200" ] && [ "$ready_code" = "200" ]; then
	log_ok "health=$health_code ready=$ready_code"
	report_push_verified "instance health + readiness (HTTP 200)"
else
	log_warn "health=$health_code ready=$ready_code — instance may not be ready"
	report_push_residual "health/ready non-200 (health=$health_code ready=$ready_code)"
fi

# ---------------------------------------------------------------------------
# 2. Topology snapshot (sites / routes / channels)
# ---------------------------------------------------------------------------
log "step 2: topology snapshot (sites, routes, channels)"
sites_code="$(curl_admin /api/sites sites.json)"
routes_code="$(curl_admin '/api/routes?view=summary' routes.json)"
channels_code="$(curl_admin /api/channels channels.json)"

site_count="$(jq 'length' "$RAW_DIR/sites.json" 2>/dev/null || echo 0)"
route_count="$(jq 'length // (.items | length) // 0' "$RAW_DIR/routes.json" 2>/dev/null || echo 0)"
channel_count="$(jq 'length // (.items | length) // 0' "$RAW_DIR/channels.json" 2>/dev/null || echo 0)"

# Count channels per route — the key multi-channel readiness signal.
# /api/routes?view=summary returns route summaries; channel counts may live in
# /api/routes/{id}/channels. Best-effort: derive from channels list grouped by routeId.
multi_channel_routes=0
max_channels_per_route=0
if [ "$channels_code" = "200" ] && [ -s "$RAW_DIR/channels.json" ]; then
	# Channels payload shape is either a bare array or {items:[...]}.
	per_route_json="$(jq '[.. | objects | (.routeId // .route_id // empty)] | group_by(.) | map({routeId: .[0], channels: length})' "$RAW_DIR/channels.json" 2>/dev/null || echo '[]')"
	max_channels_per_route="$(echo "$per_route_json" | jq '[.[].channels] | max // 0')"
	multi_channel_routes="$(echo "$per_route_json" | jq '[.[] | select(.channels > 1)] | length')"
else
	per_route_json='[]'
fi

report_set '.topology' "$(jq -n \
	--argjson sitesStatus "$sites_code" \
	--argjson routesStatus "$routes_code" \
	--argjson channelsStatus "$channels_code" \
	--argjson siteCount "$site_count" \
	--argjson routeCount "$route_count" \
	--argjson channelCount "$channel_count" \
	--argjson multiChannelRoutes "$multi_channel_routes" \
	--argjson maxChannelsPerRoute "$max_channels_per_route" \
	--argjson perRoute "$per_route_json" \
	'{
		sitesStatus: $sitesStatus, routesStatus: $routesStatus, channelsStatus: $channelsStatus,
		siteCount: $siteCount, routeCount: $routeCount, channelCount: $channelCount,
		multiChannelRoutes: $multiChannelRoutes, maxChannelsPerRoute: $maxChannelsPerRoute,
		perRoute: $perRoute
	}')"

if [ "$sites_code" = "200" ] && [ "$routes_code" = "200" ] && [ "$channels_code" = "200" ]; then
	log_ok "topology: sites=$site_count routes=$route_count channels=$channel_count"
	report_push_verified "topology snapshot captured (sites=$site_count, routes=$route_count, channels=$channel_count)"
else
	log_warn "topology endpoints incomplete (sites=$sites_code routes=$routes_code channels=$channels_code)"
	report_push_residual "topology endpoints incomplete (sites=$sites_code routes=$routes_code channels=$channels_code)"
fi

if [ "$multi_channel_routes" -gt 0 ]; then
	log_ok "multi-channel routes present: $multi_channel_routes (max $max_channels_per_route channels on one route)"
	report_push_verified "$multi_channel_routes route(s) have >=2 channels (cascade-capable topology)"
else
	log_warn "no route with >=2 channels detected — cascade has no sibling to failover to"
	report_push_residual "no multi-channel route detected — cascade failover cannot be exercised on this instance"
fi

# ---------------------------------------------------------------------------
# 3. Metrics before
# ---------------------------------------------------------------------------
log "step 3: metrics snapshot (before)"
metrics_before_code="$(curl_public /metrics metrics_before.txt)"

extract_counter() {
	# $1 = metric name. Returns the numeric sample (last match) or 0.
	awk -v m="$1" '$1==m {print $2} END{if(!found) print 0}' "$RAW_DIR/metrics_before.txt" 2>/dev/null \
		| tail -n1 || echo 0
}

requests_before="$(grep -E '^metapi_proxy_requests_total ' "$RAW_DIR/metrics_before.txt" 2>/dev/null | awk '{print $2}' | tail -n1 || echo 0)"
errors_before="$(grep -E '^metapi_proxy_errors_total ' "$RAW_DIR/metrics_before.txt" 2>/dev/null | awk '{print $2}' | tail -n1 || echo 0)"

# Labeled outcome counters (endpoint|status|stream) — capture the full block so
# we can diff after the probe. Store as a sorted blob for jq comparison.
outcomes_before_json="$(awk '/^metapi_proxy_outcomes_total\{/{found=1} found{print} /^$/{found=0}' "$RAW_DIR/metrics_before.txt" 2>/dev/null | sort || true)"
report_save_raw "$RAW_DIR/metrics_before.txt" metrics_before.txt

log "  requests_total=$requests_before errors_total=$errors_before"

# ---------------------------------------------------------------------------
# 4. Live probe — minimal chat completion
# ---------------------------------------------------------------------------
log "step 4: live probe (minimal chat completion)"

# Resolve a model to probe. Prefer the explicit override; otherwise derive
# from /v1/models (downstream-visible catalog) or the first route's model.
probe_model="$METAPI_TEST_MODEL"
if [ -z "$probe_model" ]; then
	models_code="$(curl -sS -o "$RAW_DIR/models.json" -w '%{http_code}' \
		-H "Authorization: Bearer ${METAPI_PROXY_TOKEN}" \
		--max-time "$METAPI_REQUEST_TIMEOUT" \
		"${METAPI_URL}/v1/models" 2>"$RAW_DIR/models.err" || true)"
	if [ "$models_code" = "200" ] && [ -s "$RAW_DIR/models.json" ]; then
		# /v1/models returns {data:[{id:...}]}
		probe_model="$(jq -r '.data[0].id // empty' "$RAW_DIR/models.json" 2>/dev/null || true)"
	fi
	if [ -z "$probe_model" ]; then
		# Fallback: first route's source model from the routes summary.
		probe_model="$(jq -r '.[0].sourceModel // .[0].source_model // .items[0].sourceModel // empty' "$RAW_DIR/routes.json" 2>/dev/null || true)"
	fi
fi

probe_status=""
probe_request_id=""
probe_body_excerpt=""
probe_skipped=0

if [ -z "$probe_model" ]; then
	log_warn "no probe model resolved — skipping live probe (set METAPI_TEST_MODEL to force one)"
	probe_skipped=1
	report_push_residual "live probe skipped — no model resolved (set METAPI_TEST_MODEL)"
else
	log "  probing model: $probe_model"
	probe_body="{\"model\":\"${probe_model}\",\"messages\":[{\"role\":\"user\",\"content\":\"p0585 cascade verify (read-only)\"}],\"max_tokens\":1,\"stream\":false}"
	probe_status="$(curl_proxy_json /v1/chat/completions probe.json "$probe_body")"
	if [ -f "$RAW_DIR/probe.json.headers" ]; then
		probe_request_id="$(awk -F': ' 'tolower($1)=="x-request-id"{gsub(/\r/,"",$2); print $2; exit}' "$RAW_DIR/probe.json.headers" 2>/dev/null || true)"
	fi
	if [ -f "$RAW_DIR/probe.json.body" ]; then
		probe_body_excerpt="$(head -c 240 "$RAW_DIR/probe.json.body" | tr -d '\n')"
	fi
	log "  probe status=$probe_status request_id=$probe_request_id"
fi

report_set '.liveProbe' "$(jq -n \
	--arg model "$probe_model" \
	--argjson status "$probe_status" \
	--arg requestId "$probe_request_id" \
	--arg bodyExcerpt "$probe_body_excerpt" \
	--argjson skipped "$probe_skipped" \
	'{model: $model, status: $status, requestId: $requestId, bodyExcerpt: $bodyExcerpt, skipped: $skipped}')"

if [ "$probe_skipped" -eq 0 ] && [ "$probe_status" = "200" ]; then
	log_ok "live probe succeeded (HTTP 200, request_id=$probe_request_id)"
	report_push_verified "live chat-completions probe returned 200 (happy path intact)"
elif [ "$probe_skipped" -eq 0 ]; then
	log_warn "live probe returned $probe_status — not necessarily a cascade failure (model may be unavailable upstream)"
	report_push_residual "live probe status=$probe_status (model=$probe_model) — non-200, see body in raw report"
fi

# ---------------------------------------------------------------------------
# 5. Metrics after + diff
# ---------------------------------------------------------------------------
log "step 5: metrics snapshot (after) + diff"
metrics_after_code="$(curl_public /metrics metrics_after.txt)"
requests_after="$(grep -E '^metapi_proxy_requests_total ' "$RAW_DIR/metrics_after.txt" 2>/dev/null | awk '{print $2}' | tail -n1 || echo 0)"
errors_after="$(grep -E '^metapi_proxy_errors_total ' "$RAW_DIR/metrics_after.txt" 2>/dev/null | awk '{print $2}' | tail -n1 || echo 0)"
outcomes_after_json="$(awk '/^metapi_proxy_outcomes_total\{/{found=1} found{print} /^$/{found=0}' "$RAW_DIR/metrics_after.txt" 2>/dev/null | sort || true)"

req_delta=$(( requests_after - requests_before ))
err_delta=$(( errors_after - errors_before ))

report_set '.metricsDiff' "$(jq -n \
	--argjson requestsBefore "$requests_before" \
	--argjson requestsAfter "$requests_after" \
	--argjson requestsDelta "$req_delta" \
	--argjson errorsBefore "$errors_before" \
	--argjson errorsAfter "$errors_after" \
	--argjson errorsDelta "$err_delta" \
	'{requestsBefore: $requestsBefore, requestsAfter: $requestsAfter, requestsDelta: $requestsDelta, errorsBefore: $errorsBefore, errorsAfter: $errorsAfter, errorsDelta: $errorsDelta}')"

if [ "$req_delta" -ge 1 ]; then
	log_ok "metrics moved: requests_total +$req_delta (errors +$err_delta)"
	report_push_verified "Prometheus counters moved after probe (requests +$req_delta, errors +$err_delta)"
else
	log_warn "metrics did not move — probe may not have reached the proxy loop"
	report_push_residual "metrics did not move after probe (requests_delta=$req_delta)"
fi

# ---------------------------------------------------------------------------
# 6. Cascade evidence — group recent proxy_logs by request_id
# ---------------------------------------------------------------------------
log "step 6: cascade evidence (recent proxy_logs grouped by request_id)"
logs_code="$(curl_admin '/api/stats/proxy-logs?view=query&limit=100' proxy_logs.json)"

cascaded_request_count=0
cascaded_examples_json='[]'
if [ "$logs_code" = "200" ] && [ -s "$RAW_DIR/proxy_logs.json" ]; then
	# Items is a list of proxy_log rows. Group by requestId; rows sharing a
	# request_id with >1 distinct channelId (or retry_count>0) are cascade
	# evidence. We only read fields — never mutate.
	cascaded_examples_json="$(jq -r '
		.items // []
		| group_by(.requestId // "")
		| map(select(.[0].requestId != ""))
		| map({
			requestId: .[0].requestId,
			attempts: length,
			distinctChannels: ([.[].channelId // .[0].channel_id] | unique | length),
			channels: [.[].channelId // .[0].channel_id],
			retryCounts: [.[].retryCount // .[0].retry_count // 0],
			statuses: [.[].status],
			httpStatuses: [.[].httpStatus // .[0].http_status],
			model: (.[0].modelRequested // .[0].model_requested // .[0].model // ""),
			createdAt: .[0].createdAt
		})
		| map(select(.attempts > 1 or ([.retryCounts[]] | any(. > 0))))
	' "$RAW_DIR/proxy_logs.json" 2>/dev/null || echo '[]')"
	cascaded_request_count="$(echo "$cascaded_examples_json" | jq 'length')"
else
	log_warn "proxy_logs endpoint returned $logs_code — cannot collect cascade evidence from logs"
	report_push_residual "proxy_logs endpoint unavailable ($logs_code) — no on-disk cascade evidence collected"
fi

if [ "$cascaded_request_count" -gt 0 ]; then
	log_ok "found $cascaded_request_count request(s) with multi-channel cascade attempts in recent logs"
	report_push_verified "$cascaded_request_count request(s) show multi-channel cascade attempts in proxy_logs (on-disk evidence)"
else
	log_warn "no multi-channel cascade attempts found in the last 100 proxy_logs"
	report_push_residual "no cascade attempt found in recent 100 proxy_logs — either topology is single-channel or no 5xx occurred recently"
fi
report_set '.cascadeEvidence' "$(jq -n \
	--argjson logsStatus "$logs_code" \
	--argjson cascadedRequestCount "$cascaded_request_count" \
	--argjson examples "$cascaded_examples_json" \
	'{logsStatus: $logsStatus, cascadedRequestCount: $cascadedRequestCount, examples: $examples}')"

# ---------------------------------------------------------------------------
# 7. Manual cascade-trigger procedure (documentation only — NOT executed)
# ---------------------------------------------------------------------------
log "step 7: documenting manual cascade-trigger procedure (NOT executed — read-only)"

manual_procedure='To force a cascade against this instance (destructive — do NOT run unattended):
  1. Pick a route with >=2 channels (see topology.perRoute in this report).
  2. PUT /api/channels/{id} with {"enabled":false} on the first channel only,
     OR point its account at a deliberately broken upstream.
  3. Send a chat completion for the route model; expect the conductor to
     exclude the failing channel and retry on a healthy sibling.
  4. Confirm by querying proxy_logs for the X-Request-Id: you should see
     >=2 rows with the same requestId, distinct channelId, and retryCount
     stepping 0 -> 1.
  5. Re-enable the channel and verify recovery.
This script intentionally does NOT perform step 2-3 automatically because
disabling a production channel is a destructive, non-read-only action.'
report_push_residual "manual cascade-trigger procedure documented but NOT executed (production-safe)"

# ---------------------------------------------------------------------------
# 8. Verdict + human summary
# ---------------------------------------------------------------------------
# Verdict is honest: "verified" only when both topology AND on-disk/live
# evidence were collected; otherwise "partial".
verified_count="$(jq '.verified | length' "$REPORT_JSON")"
residual_count="$(jq '.residual | length' "$REPORT_JSON")"

if [ "$cascaded_request_count" -gt 0 ] && [ "$multi_channel_routes" -gt 0 ]; then
	verdict="verified"
else
	verdict="partial"
fi
report_set '.verdict' "$(jq -n --arg v "$verdict" '$v')"

echo ""
echo "================ P0-585 cascade verification report ================"
echo " Instance      : $METAPI_URL"
echo " Timestamp     : $TIMESTAMP_ISO"
echo " Verdict       : $verdict"
echo " Verified      : $verified_count item(s)"
echo " Residual      : $residual_count item(s)"
echo " Topology      : sites=$site_count routes=$route_count channels=$channel_count multi-channel-routes=$multi_channel_routes (max $max_channels_per_route)"
echo " Live probe    : model=$probe_model status=$probe_status request_id=$probe_request_id"
echo " Metrics diff  : requests +$req_delta, errors +$err_delta"
echo " Cascade logs  : $cascaded_request_count request(s) with multi-channel attempts in last 100 proxy_logs"
echo " Report JSON   : $REPORT_JSON"
echo " Raw artefacts : $RAW_DIR/"
echo "====================================================================="
echo ""
echo "Verified:"
jq -r '.verified[] | "  - " + .' "$REPORT_JSON"
echo "Residual (honest gaps):"
jq -r '.residual[] | "  - " + .' "$REPORT_JSON" || echo "  (none)"
echo ""
echo "Manual procedure (NOT executed):"
echo "$manual_procedure"
