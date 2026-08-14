# P0-585 Production Multi-Channel Cascade Verification

**Status**: partial → verification procedure shipped; on-disk evidence collection automated; live cascade remains operator-gated.
**Issue**: [#557](https://github.com/DeliciousBuding/metapi-go/issues/557)
**Last updated**: 2026-08-15

> Companion to the cascade isolation SSOT in [`docs/architecture.md`](../architecture.md) §"Channel isolation, cooldown, circuit breaker" and the honesty row in [`docs/STATE.md`](../STATE.md) (`cascade | partial`). This document closes out the production-evidence gap referenced there.

## 1. What the P0-585 cascade is

P0-585 is the multi-channel cascade failover path inside the proxy request loop. When an upstream channel returns a retryable failure (≥500, 408/409/425/429, 401/403, or an error text matching the retryable patterns in `proxy/retry_policy.go`), the conductor **excludes that channel for the remainder of the request** and tries the next healthy sibling. The cascade is **channel-scoped**: exclusion never poisons sibling channels on the same site, and it is bounded by `ProxyMaxChannelAttempts` (env `PROXY_MAX_CHANNEL_ATTEMPTS`, default 3).

### Where the loop lives

The request loop is in `handler/proxy/upstream.go` → `dispatchUpstream`:

```304:340:handler/proxy/upstream.go
	excludeChannelIDs := make([]int64, 0)
	maxRetries := ctx.MaxRetries
```

Each attempt calls `proxy.SelectProxyChannelForAttempt` with the growing `excludeChannelIDs` list, dispatches via `dispatchSelectedUpstream`, and on a retryable failure records the failure (channel-scoped cooldown) and loops to the next sibling. The `X-Request-Id` header (set by the `WithRequestID` middleware) is **stable across all retry/failover attempts for one client call**, and every attempt writes a `proxy_logs` row tagged with that `request_id` and a stepping `retry_count`. That on-disk correlation is the primary production evidence surface.

### Decision matrix (retry policy)

`proxy.ShouldRetryProxyRequest` decides whether a failure justifies a cascade attempt:

| Upstream signal | Retryable? | Notes |
|---|---|---|
| HTTP ≥ 500 | yes | always cascade |
| 408 / 409 / 425 / 429 | yes | timeout / conflict / rate-limit |
| 401 / 403 | yes | may resolve via OAuth refresh on another channel |
| model-unsupported text | yes | try a channel that has the model |
| non-retryable request text (malformed, unknown param) | no | bad request, do not cascade |
| 400 / 404 / 422 | no | unless error text overrides |

Exclusion is **channel-only** — same-site sibling channels stay eligible. Site-scoped concerns (cooldown, site runtime breaker, credential-scoped usage-limit) are applied independently by the routing selector, never by expanding the request-local exclude list.

## 2. Existing automated evidence (before this change)

| Layer | File | What it proves |
|---|---|---|
| HTTP e2e — storm | `e2e/e2e_cascade_isolation_test.go` → `TestCascadeIsolation_MultiChannel5xxStorm_ChannelScopedExclude` | A 6-channel 5xx storm selects exactly `ProxyMaxChannelAttempts` unique channels, never re-selects an excluded channel, and the exclude snapshots grow by one channel id per failover step. |
| HTTP e2e — recovery | `e2e/e2e_cascade_isolation_test.go` → `TestCascadeIsolation_5xxThenHealthySiblingSucceeds` | A 5xx on channel 1 followed by a healthy channel 2 recovers on the sibling without re-hitting channel 1, and channel 3 is never selected. |
| Routing unit | `routing/failure_isolation_test.go`, `routing/probe_health_test.go` | Cooldown and probe failures stay channel-scoped (no sibling poison). |

> **Naming note:** the issue brief referenced `proxy/conductor_test.go` and `TestConductor_P0585LoadProof_*`. Those filenames do not exist in the as-built tree — the conductor role is split across `proxy/channel_selection.go` (`SelectProxyChannelForAttempt`), `proxy/retry_policy.go` (`ShouldRetryProxyRequest`), and `handler/proxy/upstream.go` (the retry loop). The actual load-proof tests are the two `TestCascadeIsolation_*` functions above. This doc uses the real names.

## 3. What was missing (the gap this change addresses)

`docs/STATE.md` records `cascade | partial | HTTP load proof exists; production multi-channel proof still required`. The HTTP e2e tests use `httptest.Server` mocks — they prove the code path is correct, **not** that a real production instance with a real multi-site topology actually cascades. Issue #557 requires production/staging evidence against the live deployment (the operator runs the script against the instance via `METAPI_URL`).

This change ships the **verification tooling** (a read-only shell script + a build-tagged Go test) and the **procedure** (this document) so an operator can collect that evidence without faking it. It does **not** attach captured evidence files, because:

1. The script is read-only and production-safe; it must be run by an operator who has the instance's tokens.
2. Cascade is only observable when an upstream genuinely 5xxs. A healthy instance produces no cascade rows — and we do not fabricate 5xxs against production.

## 4. Verification procedure

### 4.1 Prerequisites

- Access to the production host (SSH) or an SSH tunnel (`ssh -L 4000:127.0.0.1:4000 <prod-host>`).
- `METAPI_AUTH_TOKEN` (admin `AUTH_TOKEN`) — needed for `/api/*` (topology + proxy_logs).
- `METAPI_PROXY_TOKEN` (downstream `PROXY_TOKEN`) — needed for `/v1/*` (live probe).
- `curl` and `jq` on the machine running the script.

### 4.2 Run the verification script

The script is at `scripts/verify-cascade-prod.sh`. It is **read-only**: it checks health, snapshots topology, sends one minimal chat-completions probe, diffs Prometheus counters, and groups recent `proxy_logs` by `request_id` to surface cascade attempts. It does **not** disable channels or mutate routing state.

```bash
# Option A — run on the production host itself (instance bound to 127.0.0.1:4000):
ssh <prod-host> 'bash -s' < scripts/verify-cascade-prod.sh

# Option B — run locally against an SSH tunnel to the production host:
ssh -L 4000:127.0.0.1:4000 <prod-host> -N   # in one terminal
METAPI_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN=<admin-token> \
METAPI_PROXY_TOKEN=<proxy-token> \
METAPI_TEST_MODEL=<a-model-on-a-multi-channel-route> \
  ./scripts/verify-cascade-prod.sh
```

The script writes a structured JSON report to `./cascade-verify-reports/cascade-verify-<timestamp>.json` and raw artefacts (metrics, topology, proxy_logs) to a sibling `raw-<timestamp>/` directory. Keep both when attaching evidence to the issue.

### 4.3 Run the staging Go test (optional, deeper assertions)

The staging test is at `e2e/e2e_p0585_production_test.go` and is guarded by the `//go:build staging` tag, so it is **never** compiled into the normal CI binary. It sends one probe, captures the `X-Request-Id`, fetches recent `proxy_logs`, and asserts channel-scoped/bounded semantics **when** a cascade is observed. When the instance is healthy it `t.Skip`s with an honest residual.

```bash
METAPI_STAGING_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN=<admin-token> \
METAPI_PROXY_TOKEN=<proxy-token> \
METAPI_TEST_MODEL=<a-model-on-a-multi-channel-route> \
  go test ./e2e -tags=staging -run 'P0585_ProductionCascade_Staging' -v -timeout=120s
```

### 4.4 Triggering a cascade (operator-gated, NOT automated)

The script and the Go test do **not** force a 5xx — that would require disabling a production channel, which is destructive and out of scope for a read-only verification. To genuinely trigger a cascade against the production instance, an operator performs these steps manually (documented in the script's final report block):

1. Pick a route with ≥2 channels (the script's `topology.perRoute` field identifies these).
2. `PUT /api/channels/{id}` with `{"enabled":false}` on the **first** channel only, **or** point its account at a deliberately broken upstream.
3. Send a chat completion for the route's model; expect the conductor to exclude the failing channel and retry on a healthy sibling.
4. Confirm by querying `proxy_logs` for the `X-Request-Id`: you should see ≥2 rows with the same `requestId`, distinct `channelId`, and `retryCount` stepping `0 → 1`.
5. Re-enable the channel and verify recovery.

Steps 2–3 are the only destructive part and are intentionally left to a human.

## 5. Evidence to capture

When attaching production evidence to issue #557, capture:

| Artefact | Source | What it shows |
|---|---|---|
| `cascade-verify-<ts>.json` | script report | Verdict, topology, metrics diff, cascade-evidence summary |
| `raw-<ts>/metrics_before.txt` + `metrics_after.txt` | `/metrics` | `metapi_proxy_requests_total` / `metapi_proxy_errors_total` / labeled `metapi_proxy_outcomes_total` movement |
| `raw-<ts>/sites.json`, `routes.json`, `channels.json` | `/api/sites`, `/api/routes`, `/api/channels` | The multi-channel topology that makes cascade possible |
| `raw-<ts>/proxy_logs.json` | `/api/stats/proxy-logs` | Rows grouped by `requestId` — ≥2 rows with distinct `channelId` and stepping `retryCount` are the cascade proof |
| Staging test `-v` output | `go test -tags=staging` | `verified: cascade used N distinct channels` / `verified: cascade bounded (max retry_count=N)` lines |

The decisive signal is the last one: a `requestId` with multiple `proxy_logs` rows, distinct `channelId`s, and `retryCount` stepping — exactly the on-disk truth the HTTP e2e asserts against, observed against a live instance.

## 6. Honest residual

| Verified by this change | Still partial |
|---|---|
| Verification **procedure + tooling** shipped (script + build-tagged test). | No captured production evidence is committed; evidence must be attached by an operator run. |
| Read-only topology/metrics/proxy_logs collection is automated and safe. | Cascade is only observable on a real 5xx; a fully-healthy instance yields no cascade rows (the test honestly `t.Skip`s). |
| Staging test asserts channel-scoped exclude + bounded retry **when** cascade occurs. | The destructive trigger (channel disable) is operator-gated, not automated. |
| `go build ./...` + existing `e2e` tests stay green; the staging test is build-tagged out of CI. | If the production instance has no multi-channel route, cascade cannot be exercised on it at all — a topology gap, not a code gap. |

## 7. Related files

- `scripts/verify-cascade-prod.sh` — read-only production verification script.
- `e2e/e2e_p0585_production_test.go` — `//go:build staging` test; deeper assertions when cascade is observed.
- `e2e/e2e_cascade_isolation_test.go` — the HTTP-path load-proof tests (mock upstreams).
- `handler/proxy/upstream.go` — the cascade retry loop (`dispatchUpstream`).
- `proxy/retry_policy.go` — `ShouldRetryProxyRequest` decision matrix.
- `proxy/channel_selection.go` — `SelectProxyChannelForAttempt` (channel-scoped exclude).
- `routing/selector.go` — `SelectChannel` / `SelectNextChannel` (exclude-aware selection).
- `docs/architecture.md` — architecture SSOT (§"Channel isolation, cooldown, circuit breaker").
- `docs/STATE.md` — product honesty row (`cascade | partial`).
