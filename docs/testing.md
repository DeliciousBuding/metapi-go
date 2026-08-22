# Testing Strategy and Real-Platform Testbed

**Last verified**: 2026-08-16

> Public, environment-agnostic test guidance. Product state lives in [`STATE.md`](internal/STATE.md); open outcomes live in [`progress/MASTER.md`](internal/progress/MASTER.md).

## Test layers

| Layer                     | Command / asset                                                                      | Protects                                                                     |
| :------------------------ | :----------------------------------------------------------------------------------- | :--------------------------------------------------------------------------- |
| Go unit and integration   | `go test ./... -count=1 -race`                                                       | Package behavior, dual dialect, concurrency, handlers, routing, transforms   |
| Frontend static gates     | `bun run typecheck`, `lint`, `format:check`, `knip`, `test`, `build:check` in `web/` | Types, lint, dead code, UI contracts, production bundle                      |
| Repository e2e            | `e2e/`                                                                               | Full HTTP paths with controlled upstream fixtures                            |
| Real-service CI           | `.github/workflows/main.yml`                                                         | New API and One API detect/login/route/proxy chains in service containers    |
| Frontend acceptance       | [`../web/scripts/acceptance-e2e.mjs`](../web/scripts/acceptance-e2e.mjs) (+ [`acceptance-probe-header-quirk.mjs`](../web/scripts/acceptance-probe-header-quirk.mjs) for the fresh-site accounts-page race) | Real-browser user journeys (Playwright) against a live metapi + real upstream; operator-gated, not a PR check |
| Operator runtime evidence | `scripts/e2e/*.sh` and focused staging procedures                                    | Compatibility that requires real credentials, topology, or upstream behavior |

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
