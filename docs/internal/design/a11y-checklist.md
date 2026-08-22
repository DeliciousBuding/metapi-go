# Metapi Accessibility & Responsive Checklist

**Product**: Metapi admin
**Scope**: accessibility checklist
**Related source of truth**: `docs/internal/design/DESIGN.md`, `web/src/styles/theme.css`
**Last updated**: 2026-08-23
**Status**: living acceptance checklist; known limitations are documented, not an implicit backlog

This document records keyboard, name, contrast, and responsive expectations. Known limitations are evidence only unless promoted to [`../progress/MASTER.md`](../progress/MASTER.md) or a scoped GitHub issue.

---

## 1. Acceptance criteria

| AC                                       | Status                       | Evidence / notes                                                                                                                                                                    |
| ---------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Keyboard focus order on primary flows    | **partial / pass with debt** | Shell + shared modals: Tab reaches topbar tools, sidebar/nav, main content; Esc closes search / drawer / escape-enabled modals. Full page-form roving index not done.               |
| `aria-label` on icon-only controls       | **pass for chrome**          | Topbar icon buttons labeled; mobile nav open/close labeled; SearchModal close labeled; sidebar collapse labeled when icon-only. Page action grids still have mixed coverage (debt). |
| Contrast notes for primary text/surfaces | **documented**               | §4 below; body primary ≥ 4.5:1 both themes; muted/meta may fall near threshold.                                                                                                     |
| Responsive checklist 375 / 768 / 1280    | **documented + shell pass**  | §5; shell breakpoints via `useIsMobile` (~768) and CSS media queries; no wholesale page redesign.                                                                                   |
| axe-core live scan (authenticated routes)| **pass**                     | 2026-08-18 `bun run a11y:scan`: 15 routes (dashboard/models/sites/accounts/checkin/token-routes/proxy-logs/oauth/about/settings ×6), 0 serious/critical violations.              |
| Residual a11y debt documented            | **yes**                      | §7                                                                                                                                                                                  |

---

## 2. Keyboard focus & interaction

### 2.1 Primary shell flow (must pass)

| Step                                    | Expected behavior                                                             | Current                |
| --------------------------------------- | ----------------------------------------------------------------------------- | ---------------------- |
| Login                                   | Tab: theme tools → token field → submit → external GitHub link; Enter submits | Pass                   |
| Authenticated topbar                    | Tab order: mobile hamburger (if any) → language → appearance → theme toggle → search | Pass            |
| Search (`Ctrl/Cmd+K` or search trigger) | Focus moves to search input on open; Esc closes                               | Pass                   |
| Mobile nav                              | Hamburger opens drawer; Esc / backdrop / close button dismisses               | Pass                   |
| Sidebar collapse (desktop)              | Button remains focusable when collapsed (icon-only)                           | Pass after aria fix    |

### 2.2 Focus visibility

| Rule             | Expectation                                                                            |
| ---------------- | -------------------------------------------------------------------------------------- |
| `:focus-visible` | Visible ring on interactive chrome (buttons, close controls, nav items)                |
| Mouse users      | Prefer `:focus-visible` over always-on `:focus` to avoid sticky outlines               |
| Token            | `--ring` recipe — `focus-visible:ring-3 focus-visible:ring-ring/50` (`DESIGN.md` §2.7) |
| Hit target       | Icon-only controls ≥ ~36px (topbar already ~36)                                        |

**Current**: `.modal-close-button:focus-visible` uses primary outline. Broader global focus-ring utility is residual (not all controls share one rule).

### 2.3 Keyboard traps

| Surface              | Expected                                                    | Notes                              |
| -------------------- | ----------------------------------------------------------- | ---------------------------------- |
| Search modal         | Esc exits; Tab cycles within modal                          | **Pass** — `useFocusTrap` on panel |
| Centered modal       | Esc optional via `closeOnEscape`; close button always named | **Pass** — trap + dialog name      |
| Mobile drawer        | Esc exits; role=`dialog` + `aria-modal`                     | **Pass** — trap on panel           |
| Theme/user dropdowns | Esc dismisses non-modal menus                               | **Pass** (2026-08-18) — Base UI `DropdownMenu`/`Popover` close on Esc natively; pinned by `interface-controls.test.tsx` (language menu + appearance popover) |

### 2.4 Focus order anti-patterns (do not introduce)

1. Positive `tabIndex` > 0
2. Icon-only `<button>` / `<a>` without accessible name
3. Removing outline without a visible replacement ring
4. `pointer-events: none` on the only focusable control
5. Opening a modal without moving focus into it (search already focuses input)

---

## 3. Accessible names (`aria-label` / visible text)

### 3.1 Shell chrome (required)

| Control                  | Accessible name                      | Location                                               |
| ------------------------ | ------------------------------------ | ------------------------------------------------------ |
| Mobile hamburger         | `打开导航`                           | `web/src/components/layout/components/app-header.tsx`  |
| Language toggle          | bilingual explicit labels            | `web/src/components/layout/components/app-header.tsx`  |
| Search trigger           | `搜索 (Ctrl+K)`                      | `web/src/components/layout/components/app-header.tsx`  |
| Theme menu trigger       | mode label (+ resolved system theme) | `web/src/components/layout/components/app-header.tsx`  |
| Sidebar item (collapsed) | item label                           | `web/src/components/layout/components/app-sidebar.tsx` |
| Sidebar collapse         | `收起侧边栏` / `展开侧边栏`          | `web/src/components/layout/components/app-sidebar.tsx` |
| Mobile drawer close      | `关闭导航` (or `closeLabel`)         | `web/src/components/ui/sheet.tsx`                      |
| Modal close (×)          | `关闭弹框`                           | `web/src/components/ui/dialog.tsx`                     |
| Search modal close       | `关闭`                               | `web/src/components/ui/command.tsx`                    |
| Login GitHub icon link   | `GitHub`                             | `web/src/features/auth/components/login-form.tsx`      |
| Login theme tools group  | `外观设置`                           | `web/src/features/auth/components/login-form.tsx`      |

> 2026-08-18 hygiene: the historical `Avatar menu` row was removed — the
> current header ships no avatar/account menu (auth is token-based), so
> there is no control to name.

### 3.2 Shared component rules

1. **Icon-only button** → required `aria-label` (or `aria-labelledby`).
2. **Icon + visible text** → name may come from text; decorative SVG `aria-hidden="true"`.
3. **Close affordances** → never rely on `×` glyph alone.
4. **Collapsed rail** → every nav glyph must keep a name (`aria-label` or tooltip + label).
5. **Dynamic state** → prefer state in the name (`展开侧边栏` vs `收起侧边栏`, unread counts may stay visual if parent is named).

### 3.3 Decorative media

| Element                        | Rule                                                    |
| ------------------------------ | ------------------------------------------------------- |
| Inline SVG in labeled buttons  | `aria-hidden="true"`                                    |
| Logo mark next to product name | `alt="Metapi"` or empty alt if adjacent text duplicates |
| Status color dots              | Not sole channel; pair with text/badge                  |

---

## 4. Contrast notes (primary text / surfaces)

Ratios computed 2026-08-12 from the shipped `web/src/styles/theme.css` OKLCH values (WCAG 2.x relative luminance; token map in `DESIGN.md` §2); preset pairs re-audited and fixed 2026-08-23 (OKLCH→sRGB→WCAG pipeline, pinned by `web/src/styles/__tests__/contrast-gate.test.ts`). The light-theme primary CTA uses ink `--primary-foreground` on `--primary` (7.28:1, AAA) by design.

### 4.1 Light theme

| Pair                                                                  | Ratio   | WCAG AA body (4.5:1) | Notes                                                                                                                                                  |
| --------------------------------------------------------------------- | ------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--foreground` on `--card` / `--background`                           | 20.9:1  | Pass                 | Titles, primary values                                                                                                                                 |
| `--muted-foreground` on `--card`                                      | 6.3:1   | Pass                 | Labels / secondary text                                                                                                                                |
| `--secondary-foreground` on `--secondary`                             | 11.8:1  | Pass                 | Nested wells                                                                                                                                           |
| `--primary-foreground` on `--primary`                                 | 7.28:1  | Pass                 | Light-theme CTA — ink-on-brand (`--primary-foreground` = `oklch(0.145 0 0)` on the brand primary), AAA by design                                                          |
| White text on `--destructive`                                         | 4.8:1   | Pass                 | Errors, deletes                                                                                                                                        |
| Soft-badge text (`--success` / `--info` / `--warning` / `--destructive`) on `/10` fills | ≥ 4.5:1 | Pass                 | 12px soft badges — light `--success`/`--info`/`--warning` lightness lowered (0.53 / 0.53 / 0.62) 2026-08-14; `*-soft-fg` tokens ship the readable tone. 2026-08-23: light `--destructive-soft-fg` is now a standalone darker ink `oklch(0.5 0.2 27)` (aliasing `--destructive` only reached 3.84–3.99:1 on the tint; now 4.93–5.70 across presets) |

### 4.2 Dark theme

| Pair                                      | Ratio  | WCAG AA body (4.5:1) | Notes                   |
| ----------------------------------------- | ------ | -------------------- | ----------------------- |
| `--foreground` on `--card`                | 13.0:1 | Pass                 | Titles, primary values  |
| `--foreground` on `--background`          | 15.1:1 | Pass                 | Page canvas             |
| `--muted-foreground` on `--card`          | 7.2:1  | Pass                 | Labels / secondary text |
| `--secondary-foreground` on `--secondary` | 8.9:1  | Pass                 | Nested wells            |
| White text on `--primary`                 | 5.0:1  | Pass                 | CTA                     |
| White text on `--destructive`             | 4.6:1  | Pass                 | 2026-08-23: darkened to `oklch(0.575 0.19 25)` (was 2.77:1); attention-bell critical badge |
| Soft-badge text on `/10` fills            | ≥ 5.2:1 | Pass                | 2026-08-23: dark `--info-soft-fg` lifted to `oklch(0.72 0.12 245)` (was 3.45–3.74 on card surfaces) |

### 4.3 Contrast rules for implementers

1. Body copy and table primary cells → `--foreground` / `--muted-foreground` only.
2. Never place `--muted-foreground` on large reading blocks as primary content.
3. Status badges: solid text on soft fill (e.g. `text-success` on `bg-success/10`) — not muted gray.
4. Chart series colors resolve `--chart-1…5` CSS vars directly (recharts SVG) — no JS color extraction; verify both themes render the palette.
5. Focus rings must remain visible on both themes (`--ring` recipe).

---

## 5. Responsive checklist (375 / 768 / 1280)

Breakpoints used by product:

- **Mobile shell**: `useIsMobile` and layout CSS around **768px** (`data-layout="mobile|desktop"`).
- **Dense ops desktop**: ≥1280 typical laptop/monitor admin width.
- **375**: iPhone-class width; must not require horizontal page scroll for shell.

### 5.1 375px (mobile)

| Check         | Expected                                                            | Status                   |
| ------------- | ------------------------------------------------------------------- | ------------------------ |
| Topbar        | Hamburger + logo + compact tools; search may iconify                | Pass (shell)             |
| Sidebar       | Hidden; content via mobile drawer                                   | Pass — `components/ui/sidebar.tsx` renders itself as a `Sheet` drawer on mobile |
| Main padding  | Reduced; no clipped primary CTA                                     | Pass / page debt         |
| Tables        | Card/list alternative or horizontal scroll inside table region only | `MobileCardList` (`data-table/layout/mobile-card-list.tsx`) auto-swaps at ≤640px for every `DataTablePage` consumer; remaining pages debt |
| Batch actions | Floating selection bar reachable on mobile                          | Pass — shared `data-table/toolbar/bulk-actions.tsx` fixed bottom-center bar (sites / accounts / token-routes); no width-specific variant needed |
| Filters       | Toolbar filters usable at narrow widths                             | Pass — single `flex-wrap` toolbar row (`data-table/toolbar/toolbar.tsx`: search / faceted filter / view options) wraps instead of moving into a sheet |
| Touch targets | ≥36–44px for chrome icons                                           | Pass topbar              |
| Safe areas    | Avoid fixed bars covering primary content                           | Residual on some pages   |

### 5.2 768px (tablet / breakpoint edge)

| Check            | Expected                                       | Status            |
| ---------------- | ---------------------------------------------- | ----------------- |
| Layout switch    | Mobile drawer path active at ≤768              | Pass              |
| Topbar density   | Tools remain usable without overlap            | Pass              |
| Modals           | Max-width constrained; close control reachable | Pass shared modal |
| Two-column forms | Stack at narrow widths where used               | Partial adoption — per-form responsive grid classes (`sm:/md:grid-cols-2`, e.g. site form dialog, model-tester form, several settings sections); no shared grid component |

### 5.3 1280px (desktop)

| Check               | Expected                                    | Status    |
| ------------------- | ------------------------------------------- | --------- |
| Sidebar             | Expanded 220px default; collapsible to 64px | Pass      |
| Collapsed rail      | Icon-only items named                       | Pass      |
| Tables              | Full columns; sticky header optional        | Page debt |
| Topbar nav + search | Visible labels where designed               | Pass      |
| KPI + charts        | No overflow of card grid                    | Partial   |

### 5.4 Responsive anti-patterns

1. Hiding critical actions with `display: none` and no mobile equivalent.
2. Desktop-only hover menus without a tap path.
3. Fixed pixel widths that force document-level horizontal scroll at 375.
4. Relying on tooltip-only labels when the rail collapses.

---

## 6. Reduced motion & semantics

| Topic                                  | Expectation                                                                                                                     | Status                                                                                                                                                          |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `prefers-reduced-motion: reduce`       | Collapse non-essential transitions/animations                                                                                   | **Pass** — token durations → ~0 in `theme.css`; global hard-cut `animation/transition-duration` in `web/src/styles/index.css`                                   |
| `prefers-reduced-transparency: reduce` | Glass → solid elevated; strip backdrop blur                                                                                     | **Pass** (2026-08-21) — `@media (prefers-reduced-transparency: reduce)` in `web/src/styles/index.css`: the shared glass recipe (topbar + floating bulk-actions bar: `.backdrop-blur-lg` over translucent `bg-background/*`) and the dialog/sheet/alert-dialog scrims (`bg-overlay` + `backdrop-blur-xs`) lose backdrop blur and solidify to opaque `--background`; borders/shadows untouched. Toasts already sit on opaque `--popover`; sidebar and sign-in are flat, so no fallback needed there. |
| Dialog semantics                       | `role="dialog"` + `aria-modal` for blocking overlays                                                                            | Mobile drawer pass; SearchModal improved; not all legacy overlays                                                                                               |
| Live regions                           | Toasts/errors announced                                                                                                         | **Partial** (2026-08-22): form errors are now announced — the shared `FormMessage` primitive carries `role="alert"` (added by #920, pinned by `web/src/components/ui/__tests__/form-message.test.tsx`); toast announcements remain residual (sonner toasts have no live-region wiring yet). |
| Language                               | `t()` for user-visible chrome strings; `aria-label` included in i18n attr list                                                  | Pass pattern                                                                                                                                                    |
| Page heading structure                 | One `h1` per settings section page — breadcrumb header (`Settings / subarea`) + section card owns the single `h1` + description | **Pass** (2026-08-12): `settings-page.tsx` + `settings-section-card.tsx`; verified via Playwright (1 h1 on maintenance / danger-zone / import-export)           |

---

## 7. Known limitations

This section lists open residuals only. Closure history lives in [`../log.md`](../log.md) and root [`../../CHANGELOG.md`](../../../CHANGELOG.md). Open work is committed through [`../progress/MASTER.md`](../progress/MASTER.md) or a scoped issue.

1. **Charts keyboard series access** — recharts renders series as non-focusable SVG; assistive tech relies on the text axes, legends, and rich text tooltips (balance/cost, accounts, calls, tokens, share) that already carry the data. Non-color status encoding (text labels on availability buckets, attention badges) is in place; no color-only status.
2. **Global focus-ring utility** — chrome controls share the `--ring` recipe; a single shared rule for every page-level action grid is not yet in place (`.modal-close-button:focus-visible` uses a primary outline).
3. **Hex hygiene** — no new brand hex is allowed in pages (see [`DESIGN.md`](./DESIGN.md) §1 Principles). Existing brand assets and other justified exceptions are reviewed when their owning surface changes; this is not a standalone sweep.
4. **Preset contrast** — 2026-08-23: a static WCAG audit (OKLCH→sRGB→WCAG ratio pipeline) found and **fixed** six sub-AA defects; all fixed pairs are pinned by `web/src/styles/__tests__/contrast-gate.test.ts`. New baselines: rose-garden dark `--secondary` 1.50→7.64 (dark rose surface `oklch(0.4 0.08 8)`); default dark solid `--destructive` 2.77→4.61 (`oklch(0.575 0.19 25)`, attention-bell critical badge); white CTA text on five preset dark primaries — forest-whisper 3.59→4.81, ocean-breeze 3.68→4.72, lavender-dream 3.68→4.72, rose-garden 3.68→4.71, sunset-glow 3.69→4.72 (L lowered 0.06–0.07); anthropic light clay `--primary` 2.91→4.64 (`oklch(0.57 0.15 38)`) and sky `--info` 2.85→4.62 (`oklch(0.55 0.075 248)`); soft-badge foregrounds — light `--destructive-soft-fg` standalone `oklch(0.5 0.2 27)` (3.84–3.99→4.93–5.70) and dark `--info-soft-fg` `oklch(0.72 0.12 245)` (3.45–3.74→5.17–5.70); sidebar active item — default light `--sidebar-accent-foreground` 3.95→5.38 and the lake-view dark pure-black override deleted (1.75→8.95). Remaining documented sub-AA preset residuals (exemption list in the gate): forest-whisper dark `--secondary` 3.12, ocean-breeze light `--secondary` 4.47, simple-large/anthropic dark bespoke `--destructive` 2.97/2.79, anthropic light olive `--success` 3.83 (soft 4.48); the `bg-sidebar-primary` pairs (3.40/4.11) are a dormant token no component consumes.

---

## 8. Manual test script (release smoke)

Run against both light and dark themes.

### 8.1 Keyboard

1. Login with keyboard only; confirm error text is readable.
2. Tab through topbar; open search; type query; Esc closes; focus returns to trigger (ideal) or remains usable.
3. Toggle theme menu; select Dark/Light/System.
4. Desktop: collapse sidebar; Tab to icon rail; confirm names via screen reader or accessibility inspector.
5. Mobile width 375: open nav drawer; Esc closes; navigate to Sites.

### 8.2 Names

1. Accessibility tree: no unlabeled buttons in topbar/sidebar/search header.
2. Modal × announces close.
3. Search close announces close.

### 8.3 Contrast

1. Dashboard KPI labels and table body in both themes.
2. Danger/success badges on soft fills remain readable.
3. Placeholder text is allowed to be muted; form labels must not be.

### 8.4 Responsive

1. **375**: login, dashboard, one table page (Accounts or Sites), search modal.
2. **768**: drawer vs sidebar switch; no overlapping topbar controls.
3. **1280**: expanded sidebar, full tables, modals centered with margin.

---

## 9. Maintenance contract

- Shared accessibility behavior belongs in `web/src/components/ui/**` or the shell owner, with focused component coverage.
- Feature-specific names, labels, error announcements, and responsive fallbacks stay with the feature.
- Re-run the relevant keyboard/manual checks plus `bun run a11y:scan` when a shared interaction primitive changes.
- Do not create a permanent “sweep” project. Promote a verified defect to `MASTER.md` or an issue with a concrete owner and acceptance criterion.
