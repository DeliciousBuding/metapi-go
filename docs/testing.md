# Testing Strategy and Real-Platform Testbed

**Last verified**: 2026-08-29

> Public, environment-agnostic test guidance. Product state lives in [`STATE.md`](internal/STATE.md); open outcomes live in [`progress/MASTER.md`](internal/progress/MASTER.md).

## Test layers

| Layer                     | Command / asset                                                                      | Protects                                                                     |
| :------------------------ | :----------------------------------------------------------------------------------- | :--------------------------------------------------------------------------- |
| Go unit and integration   | `go test ./... -count=1 -race`                                                       | Package behavior, dual dialect, concurrency, handlers, routing, transforms   |
| Frontend static gates     | `bun run typecheck`, `lint`, `format:check`, `knip`, `test`, `build:check` in `web/` | Types, lint, dead code, UI contracts, production bundle                      |
| Repository e2e            | `e2e/`                                                                               | Full HTTP paths with controlled upstream fixtures                            |
| Real-service CI           | `.github/workflows/main.yml`                                                         | New API, One API, Sub2API and CLIProxyAPI detect/login/route/proxy chains in service containers, plus cross-OS binary runtime smoke (`runtime-smoke-matrix`) |
| Frontend acceptance       | [`../web/scripts/acceptance-e2e.mjs`](../web/scripts/acceptance-e2e.mjs) (+ [`acceptance-probe-header-quirk.mjs`](../web/scripts/acceptance-probe-header-quirk.mjs) for the fresh-site accounts-page race) | Real-browser user journeys (Playwright) against a live metapi + real upstream; operator-gated, not a PR check |
| Operator runtime evidence | `scripts/e2e/*.sh` and focused staging procedures                                    | Compatibility that requires real credentials, topology, or upstream behavior |

## Race-detector budget (local push gate)

The pre-push gate (`scripts/go-race.sh`, chained from
`.githooks/pre-push-project`) runs the full suite under `-race` with a
per-package timeout. The default is **900s**; override it with
`METAPI_RACE_TIMEOUT_SECONDS=<seconds>` when a host needs more headroom.

Why 900s (measured, August 2026):

- Six independent development lanes hit the old 300s default on
  `handler/admin` while parallel lanes contended for an 8GB WSL VM on a slow
  `/mnt/d` 9p mount. Isolated measurements of `handler/admin` under
  `-race` range **217-364s** depending on host load; the master baseline is
  equally critical.
- CI gives each package ~10 minutes (Go's default `go test` timeout) inside
  30-minute shard jobs, so 900s matches both CI headroom and the measured
  worst cases.

Operational guidance:

- Stagger push gates: keep race lanes to **4 or fewer concurrent** on one dev
  host. Beyond that, per-package times inflate and gates time out for
  environmental reasons, not code regressions.
- On a timeout the gate prints a one-line hint; raise the knob (for example
  `METAPI_RACE_TIMEOUT_SECONDS=1500`) rather than skipping the gate.
  `git push --no-verify` is emergency-only.

CI is unaffected by this default: `.github/workflows/main.yml` never calls
`go-race.sh`. It shards packages across four runners and runs `go test
-race` with Go's default 10-minute per-package timeout, so raising the local
gate default cannot weaken CI.

Shard selection is guarded rather than trusted, because a required check that
runs nothing still reports green. Each `test-sqlite-shard` job binds
`matrix.shard` / `matrix.total` through the environment, computes its
round-robin slot in a standalone `$(( ))` assignment, and then asserts the
number of packages it owes — `ceil((N - S) / T)` over `N = go list ./...`. A
shard that selects fewer than it owes, or none at all, fails loudly. The
slot computation is deliberately *not* an `if (( ... ))` condition: a command
in an `if` condition is exempt from `set -e`, so a malformed selector there is
swallowed instead of aborting the step.

That guard is load-bearing, not decorative. The revision that introduced the
shard matrix inlined the matrix values into the arithmetic without `${{ }}`,
so bash saw `i % matrix.total`, errored once per package, selected nothing,
and took the `no packages in shard; nothing to run` branch to `exit 0`. All
four shards and the aggregated `test-sqlite` required check reported success
while executing zero tests, from v0.14.0 through v0.17.0. `test-pg` (which
runs `go test ./...` unsharded) was the only job actually executing the Go
suite in that window, and it does not pass `-race`.

## Golden snapshot suites (protocol conversion)

Multi-protocol conversion is the most regression-prone surface of the proxy, so
it is additionally pinned by checked-in golden snapshots (modeled after the
`relaykit/relayconvert` golden pattern). Every golden case is a pair of files
under the owning package's `testdata/golden/<category>/` directory:

- `<name>.input.json` (or `.input.sse` for raw stream fixtures) — the fixture,
  hand-written and never auto-rewritten.
- `<name>.golden.json` (or `.golden.sse`) — the recorded output snapshot.

Categories follow the surface a package owns: `request`, `response`, `stream`
fixtures, plus `decision` (policy truth tables) and `unit` (leaf helpers).

| Package | Golden coverage |
| :--- | :--- |
| `transform/openai/responses` | Responses request sanitization (continuity `previous_response_id` policy, reasoning input items, compact mode) + decision tables |
| `transform/gemini/generate_content` | OpenAI→Gemini request conversion (tool calls + thought signatures, multimodal placeholders, thinking config) + Gemini request normalization |
| `transform/openai/completions`, `transform/openai/embeddings`, `transform/openai/images` | Pass-through identity contract (request/response/stream bytes unchanged) |
| `transform/shared` | Leaf helper edge cases |
| `handler/proxy` | Response-body usage extraction and incremental SSE stream parsing (usage/finish/error/done state, chunk-boundary independence). In this codebase the response/stream half of protocol handling lives here: SSE streams are relayed byte-for-byte and only parsed for accounting. |

Run the suite like any other test (it is part of `go test ./...` and therefore
part of CI):

```bash
go test ./transform/... ./handler/proxy/... -count=1
```

To regenerate snapshots after an intentional behavior change, set
`GOLDEN_UPDATE=1`:

```bash
GOLDEN_UPDATE=1 go test ./transform/... ./handler/proxy/... -count=1
```

Regeneration discipline:

- Regenerate only after deliberately deciding the new behavior is correct; the
  diff of rewritten `*.golden.*` files is the review artifact.
- Never regenerate to silence a failure you cannot explain.
- Fixture (`*.input.*`) files are data, not generated output: edit them by
  hand to add coverage.
## Public testbed assets

- [`../testbed/compose.template.yml`](../testbed/compose.template.yml): sanitized loopback-only service template for Metapi, New API, One API, and optional adapter targets.
- [`../scripts/e2e/smoke.sh`](../scripts/e2e/smoke.sh): idempotent password-login chain from health and site detection through `/v1` proxying.
- [`../scripts/e2e/verify-token-import.sh`](../scripts/e2e/verify-token-import.sh): equivalent chain for session JWTs, API keys, and management keys.
- [`analysis/p0585-production-verification.md`](internal/analysis/p0585-production-verification.md): operator-gated multi-channel cascade evidence procedure.

## Run a real-platform chain

1. Copy `testbed/compose.template.yml` into an operator-controlled test environment.
2. Put real credentials in an ignored `.env`; do not edit them into the template or scripts.
3. Start only the services needed for the adapter under test.
4. Run one of the environment-driven chains:

```bash
METAPI_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN='replace-me-admin-token' \
UPSTREAM_URL=http://127.0.0.1:3001 \
UPSTREAM_USERNAME='replace-me-username' \
UPSTREAM_PASSWORD='replace-me-password' \
PLATFORM=new-api \
  bash scripts/e2e/smoke.sh
```

```bash
METAPI_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN='replace-me-admin-token' \
UPSTREAM_URL=https://upstream.example.com \
UPSTREAM_TOKEN='replace-me-upstream-token' \
PLATFORM=sub2api \
  bash scripts/e2e/verify-token-import.sh
```

Both scripts print PASS/FAIL/WARN summaries, preserve truncated failure evidence, and exit non-zero on a failed required step.

## Privacy and evidence boundary

- The public repository may contain templates, placeholder URLs, scripts, and reproducible failure descriptions.
- Hostnames, host filesystem layouts, SSH commands, real credentials, and private upstream addresses stay in the operator-controlled environment.
- Attach sanitized request/response shapes and commit/PR references to public issues; never attach raw environment files or full tokens.
- A healthy run is evidence for the exercised platform and version only. Do not generalize it into a cluster-wide or all-adapter claim.
