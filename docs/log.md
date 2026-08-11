# log.md — MetAPI Go product milestones

**Last updated**: 2026-08-11

> Product milestone timeline (grouped by version). Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../CHANGELOG.md)

## 2026-08-11 — post-v0.9.0 UI completion batch

- **Brand rename → MetAPI**: display name unified (identity-branding / locales / About / index title); transparent SVG badge `logo.svg` (gradient rounded-square + real π glyph) + `favicon.svg` replace the white-background PNG; router root-file whitelist + table-driven regression test extended to `image/svg+xml`
- **i18n language switcher**: header `LanguageSwitcher` dropdown (en/zh-CN) + browser auto-follow (localStorage → navigator) + `documentElement.lang`/`dir` sync via `toBcp47`; locale parity now 1381 keys each, bidirectional 0 missing
- **URL-synced tables fix**: sites/models/oauth/site-announcements read the router location (`useLocation` + `searchStr`) so sort/pagination now update the table in place instead of waiting for an unrelated re-render
- **Copy audit**: terminology unification (启用/停用, 额度, Check-in, 通道), internal plan codes (K1a/N9a) removed from user-visible copy, tokenRoutes toast/chain-banner concatenation bugs fixed, 9 hardcoded strings → t() (incl. TokenDance brand leak removed from the public settings copy)
- **Visual polish**: sign-in real logo mark + brand glow + lg CTA; dashboard StatCard skeletons + useId-unique gradients + iconized empty/error states + pulsing WS indicator; settings responsive drill-in + sticky sidebar; authenticated-layout scroll-clip fix (content taller than viewport was unscrollable)

## 2026-08-11 — v0.9.0 frontend rewrite

- **Frontend rewrite**: newapi stack 100% alignment — Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH tokens (details in root `CHANGELOG.md`)
- **i18n key-based**: i18next + en/zh-CN locales each 1369 keys, bidirectional 0 missing; MutationObserver dictionary + `tr()` sweep retired, replaced by vitest `i18n-keys.test.ts` gate (1151 keys scanned)
- **Tooling**: npm → Bun (CI/Dockerfile/Makefile); Playwright + `ui-visual` removed; old frontend tree (`App.tsx`/pages/components/styles/e2e) deleted

## 2026-08-02 — v0.8.46 → v0.8.52

- **PG dialect hardening**: 4 bare `*sqlx.DB` `?` placeholders hitting PG directly (SQLSTATE 42601) all wrapped with `db.Rebind`; added static check `pg_rebind_gate_test.go` to prevent regressions
- **v0.8.49+**: `sc2_008` migration `BOOLEAN DEFAULT` PG 42804 and balance_history UPSERT 42601 — two PG dialect blind spots fixed; CI PG integration tests serialized (`-p 1`)
- **Frontend i18n sweep**: 414 bare Chinese JSX text nodes wrapped with `tr()`; added `i18n.gate.test.tsx` static check
- **Chart series color token wiring**: VChart canvas series colors — 27 `var(--color-chart-N)` silent fallbacks → `useChartColors()` JS lookup + chart-colors check
- **Design token unification**: RealtimeOpsPanel real-time traffic badge hardcoded rgba → `--color-*-soft` token
- **Feishu notifications**: TaskTag signature aggregation anti-spam (same-class alerts merged within cooldown window) + notification channel save pre-validation
- **Resource integrity three layers**: dist self-check + CD build verification + deploy live smoke test
- **Login page refactor**: single-card layout + dark de-gradient; root static files (logo/favicon) SPA fallback image-swallow fix

## 2026-07-31 — Feature batch

- **All features shipped**: model redirect mapping, snapshot PNG, risk banner, tag system, batch validation, chart gallery, randomized window scheduling, backup import preview, scheduler task run history
- **Downstream key hardening**: downstream key IP allowlist/blocklist (security gap), public price endpoint, inference suffix parsing, spend distribution dashboard, CSV export, configurable cache multiplier; remaining items honestly deferred
- **Frontend UX pass**: RouteErrorBoundary, SearchModal real keyboard navigation, Toast a11y, design-system state trio (Empty/Loading/Error), Models→Playground quick jump, ProxyLogs date presets

## 2026-07-30 — Engineering optimization + parity review

- **Package boundary enforcement**: `docs/package_boundary_test.go` turns BACKEND.md §2.3 eight hard rules into a `go test` check; caught and remediated `scheduler/lease.go` stale exception on the spot
- **Product parity review**: Go rewrite lost no TS README product features; 14 platform adapters TS=Go aligned; multi-user/payments/redemption-codes/invitations/subscriptions explicitly not applicable
- **Dual-dialect encapsulation**: `store.DB` gained Context methods (rebind `?`→`$N`), removed 4 manual dialect branches

## 2026-07-20 — v0.8.45 RE2-safe

- **RE2 panic fix** (production crash root cause): NewAPI user-id extract compiled PCRE lookahead `(?!\d)` → Go RE2 panic; switched to pre-compiled regex + 8-digit length cap
- **Four-track original feature alignment**: frontend 18 routes + 14 sidebar 100% parity; 14 platform adapters fully aligned; 16 scheduler tasks covered; WS/Sticky/UC explicitly residual
- **Original parity plan** (ex-Electron): WS full TS parity, sticky single-instance honesty, UC hide/external

## 2026-07-19 — UI polish milestone

- **UI polish batch**: Traffic sparkline, real page scoring, axe a11y smoke, Dashboard getting-started, Sites banner
- **UI parity inventory**: sidebar 18 routes at parity, Sites/Accounts/Tokens/Routes/Settings button counts at parity
- **Focus management shared**: `useFocusTrap` wired into SearchModal / CenteredModal / MobileDrawer / NotificationPanel; skip-link accessibility jump
