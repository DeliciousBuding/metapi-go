# Archive — 2026-07

Historical one-shot analysis files moved here to slim the live `docs/analysis/`
surface (neat-freak: 先减后加). These are kept for provenance, not day-to-day
use. **Do not delete** — they record decisions and evidence that may be cited.

## What was archived and why

| File | Origin | Reason | Live reference? |
|:-----|:-------|:-------|:----------------|
| `p4-account-verify.md` | P4 phase (#183, 2026-07-17) | Phase complete; no live refs (grep 全仓 0) | none |
| `p4-settings-proxy-test.md` | P4 phase (#184) | Phase complete; no live refs | none |
| `p4-token-adapter-wiring.md` | P4 phase (#182, 2026-07-17) | Phase complete; no live refs | none |
| `ui-score-2026-07-19.md` | UI visual score, design gallery (2026-07-19) | One-shot pixel score; superseded by `ui-visual-acceptance.md` | none |
| `ui-score-shell-2026-07-19.md` | UI visual score, login+gallery (2026-07-19) | One-shot; superseded | none |
| `ui-score-shell-mock-2026-07-19.md` | UI shell chrome mock (#538/#543) | One-shot; superseded | none |
| `ui-pm-empty-state-2026-07-19.md` | PM/UX empty-DB shell notes (2026-07-19) | One-shot; superseded | none |

## NOT archived (still live)

These dated/phase files were **kept** because they have live references:

| File | Why kept | Live refs |
|:-----|:---------|:----------|
| `p4-admin-test-routes.md` | admin test harness residual (501 honesty) | `admin-channel-test-harness.md:111` · `ops-admin-stubs.md:25` · `handler/admin/test.go:19` |
| `ui-score-pages-2026-07-19.md` | referenced by visual acceptance harness | `ui-visual-acceptance.md:101` |

## Archive policy

- Archived files are **not** deleted; they stay here for provenance.
- The live `docs/analysis/` directory should only contain docs referenced by
  `STATE.md` / `MASTER.md` / `README.md` / `high-value-next.md` / `residual-next-candidates.md`
  or actively guiding implementation.
- See `docs/analysis/engineering-optimization-2026-07-30.md` §2.4 for the
  neat-freak reasoning behind this archive pass.
