# E2E Acceptance Platform — layered real-testing architecture

**Status**: design + first implementation landed
**Date**: 2026-08-20
**Companions**: [`docs/testing.md`](../../testing.md) (public layers) · [`scripts/e2e/smoke.sh`](../../../scripts/e2e/smoke.sh) · [`scripts/e2e/verify-token-import.sh`](../../../scripts/e2e/verify-token-import.sh) · [`web/scripts/acceptance-e2e.mjs`](../../../web/scripts/acceptance-e2e.mjs) · [`e2e/`](../../../e2e/)

This document is the single source of truth for **how metapi-go is acceptance-tested end to end against real upstream platforms**, and — critically — **why the heavy real-environment E2E stays out of the blocking PR CI** so PR turnaround stays fast.

---

## 1. The concern this answers

> "If we put heavy E2E into CI, won't it make the pipeline very slow?"

**Yes — so we don't.** The design splits E2E into two regimes:

| Regime | Where it runs | Blocks PRs? | Why |
|:---|:---|:---|:---|
| **Deterministic real-platform E2E** | GitHub Actions, ephemeral service containers | ✅ Yes (already does) | Self-contained, no secrets, free on runners, bounded runtime |
| **Real-environment acceptance** | Operator machine → standing upstream host | ❌ No (operator-gated) | Needs live credentials + a real upstream; slow + environment-dependent |

The PR pipeline already runs the first regime (`test-e2e` job boots **real** new-api + one-api containers and drives the full chain). That gives every PR real-platform coverage **without** external dependencies. The second regime is the deeper acceptance layer and is deliberately **not** a required check.

---

## 2. Test layers (bottom → top)

| Layer | Asset | Real upstream? | Blocks PR? | Protects |
|:---|:---|:---|:---|:---|
| Go unit + integration | `go test ./... -race` | no (fixtures) | ✅ | package behavior, dual dialect, concurrency |
| Full-binary Go e2e | `e2e/` | controlled fixtures | ✅ | HTTP paths, backup, cascade, shutdown |
| Frontend static + unit | `bun run typecheck/lint/test/knip` | no | ✅ | types, UI contracts, dead code |
| Browser crash/a11y smoke | `web/scripts/route-smoke.mjs`, `a11y-scan.mjs` | fake site | ✅ | no renderer exceptions, axe, route render |
| **Real-platform CI e2e** | `test-e2e` job + `scripts/e2e/smoke.sh` | **real new-api + one-api containers** | ✅ | detect→login→checkin→route→`/v1` relay |
| **Real-environment acceptance (backend)** | `scripts/e2e/smoke.sh` + `verify-token-import.sh` against a standing host | **real deployed upstream** | ❌ | the same chain against a live, non-container upstream |
| **Real-environment acceptance (frontend)** | `web/scripts/acceptance-e2e.mjs` | **real backend + real upstream** | ❌ | actual user journeys through the built SPA |

---

## 3. Real-environment acceptance (the new layer)

A **standing upstream host** (a real new-api deployment) is kept available so acceptance can run against something that is *not* an ephemeral container. metapi is deployed as a throwaway instance pointed at it, and both the backend chain and the frontend journeys run against that pair.

**Proven against a real new-api deployment (2026-08-20):**

- Backend chain via `smoke.sh`: **13/13 PASS** — health, admin auth, platform detect, site create, **real login**, verify-token, account, models, balance, **check-in**, downstream token, route, `/v1` relay. Check-in correctly reports the honest upstream result.
- Frontend journey via `acceptance-e2e.mjs`: **site onboarding PASS** — a real Chromium drives the built SPA to add a site pointed at the real upstream and the site lands in the table. The journey selects the platform explicitly so create does not depend on auto-detect timing; the detect round-trip fires when the platform is left to auto-detection (`POST /api/sites/detect`, also run in the dialog on URL paste).
- Frontend **account-login journey** (add account → password mode → real upstream login → account appears) is implemented and **proven working**; it is opt-in (`ACCEPT_LOGIN=1`) because running it immediately after a *freshly created* site hits a transient accounts-page header overlap (a real UI quirk, tracked separately). Against a settled site it passes.

### How to run it

```bash
# 1. Deploy a throwaway metapi pointed at the standing upstream, e.g. on port 4000.
# 2. Backend chain:
METAPI_URL=http://127.0.0.1:4000 METAPI_AUTH_TOKEN=<admin> \
UPSTREAM_URL=<real-upstream> UPSTREAM_USERNAME=<user> UPSTREAM_PASSWORD=<pass> \
PLATFORM=new-api bash scripts/e2e/smoke.sh

# 3. Frontend journeys (needs a Chromium install: bunx playwright install chromium):
BASE_URL=http://127.0.0.1:4000 AUTH_TOKEN=<admin> \
UPSTREAM_URL=<real-upstream> UPSTREAM_USERNAME=<user> UPSTREAM_PASSWORD=<pass> \
  bun run --cwd web acceptance:e2e           # site-onboarding journey
ACCEPT_LOGIN=1 ... bun run acceptance:e2e    # + account-login journey
```

Note the invocation shape: `bun run --cwd web acceptance:e2e`. The reversed
form (`bun --cwd web run acceptance:e2e`) prints usage help and exits 0
without running the journeys on bun 1.x — a silent no-op, not a pass.

Credentials are supplied by the operator environment; they are **never** committed.

**Safety**: `acceptance-e2e.mjs` starts by wiping ALL sites and accounts on
its target (that is how runs stay idempotent) and defaults to
`BASE_URL=http://127.0.0.1:4000`. Always pass `BASE_URL`/`AUTH_TOKEN`
explicitly and make sure the target is the dedicated throwaway instance —
whatever listens on the default port gets wiped.

---

## 4. Why the heavy layer is operator-gated, not PR-blocking

1. **Runtime.** The PR pipeline's long pole is already the race-enabled full Go suite (sharded) plus the container e2e. Adding a live-environment browser+backend acceptance would add minutes and make every PR wait on an external host.
2. **Determinism.** A standing upstream can be rate-limited, mid-upgrade, or briefly down — fine for scheduled/manual acceptance, unacceptable as a required PR check.
3. **Secrets.** Real upstream credentials must not live in CI; they stay in the operator environment.
4. **Evidence boundary.** Real-environment runs produce sanitized evidence (verdict + request/response shapes), not raw host/credential detail.

The deterministic container `test-e2e` job already guarantees real-platform behavior on every PR; the operator-gated layer adds depth (live upstream, real browser journeys) on demand.

---

## 5. Extension roadmap

- **Journey coverage**: add frontend journeys for import (`settings/content/import-export`), check-in UI, and the migration UI, mirroring the account-login journey structure.
- **Stabilize the login journey**: fix the freshly-created-site header overlap so the account-login journey runs chained by default.
- **Scheduled acceptance**: run the backend chain + journeys on a schedule/manual trigger against the standing host and archive sanitized reports.
- **Second upstream**: stand up a real one-api alongside new-api and parameterize `PLATFORM`.

## 6. Known quirk (tracked)

When the account-login journey runs immediately after the site-onboarding journey creates a **fresh** site in the same process, the accounts-page header transiently overlaps the toolbar and intercepts the "Add account" click. Workaround: `ACCEPT_LOGIN=1` runs it in its own browser against a settled site. Root-causing the overlap is a small frontend fix, not a test bug.

**v0.16.6 re-probe (2026-08-22, `web/scripts/acceptance-probe-header-quirk.mjs`)**: the visual overlap itself was **not reproduced**. The probe recreates the original conditions (create a fresh site via API → navigate to `/accounts` immediately, no snapshot polling) and samples at 20 Hz what element sits at the "Add account" button center while attempting a real click, across 25+ trials. Findings:

- Zero load-time interceptions: no sample ever hit-tested a non-button element over "Add account" before a dialog existed; the account create Sheet (Base UI popup) is never mounted during page load. Every interception sample occurred *after* the probe's own successful click opened the create sheet — expected coverage, not the quirk.
- The measurable transient failure right after a fresh site is **slow button actionability**: the button renders/enables only ~0.4–2.3 s after navigation (page hydration + accounts snapshot arrival), so an immediate click with a short budget times out. `disabled={sites.length === 0}` on the button pins it until the snapshot lists the site — exactly what the journey's `waitForSiteInSnapshot` guard absorbs.
- Conclusion: the overlap as described was either fixed between 2026-08-20 and v0.16.6 or needs real-upstream latency to surface; the snapshot race remains the concrete, reproducible race and the journey's existing workaround is correct. The probe script is kept for regression re-runs.
