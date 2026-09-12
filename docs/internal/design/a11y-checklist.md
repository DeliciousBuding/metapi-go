# Metapi Accessibility & Responsive Checklist

**Product**: Metapi admin
**Scope**: accessibility checklist
**Related source of truth**: `docs/internal/design/DESIGN.md`, `web/src/styles/theme.css`
**Last updated**: 2026-09-12
**Status**: living acceptance checklist; known limitations are documented, not an implicit backlog

This document records keyboard, name, contrast, and responsive expectations. Known limitations are evidence only unless promoted to a scoped GitHub issue.

---

## 1. Acceptance criteria

| AC                                       | Status                       | Evidence / notes                                                                                                                                                                    |
| ---------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Keyboard focus order on primary flows    | **partial / pass with debt** | Shell + shared modals: Tab reaches topbar tools, sidebar/nav, main content; Esc closes search / drawer / escape-enabled modals. Full page-form roving index not done.               |
| `aria-label` on icon-only controls       | **pass for chrome**          | Topbar icon buttons labeled; mobile nav open/close labeled; SearchModal close labeled; sidebar collapse labeled when icon-only. Page action grids still have mixed coverage (debt). |
| Contrast notes for primary text/surfaces | **documented**               | §4 below; body primary ≥ 4.5:1 both themes; muted/meta may fall near threshold.                                                                                                     |
| Responsive checklist 375 / 768 / 1280    | **documented + shell pass**  | §5; shell breakpoints via `useIsMobile` (~768) and CSS media queries; no wholesale page redesign.                                                                                   |
| axe-core live scan (authenticated routes)| **pass**                     | CI `a11y` job, on every PR and every master merge: serves the real embedded SPA from the Go binary (fresh sqlite runtime DB) and scans the shared route inventory — `DESKTOP_ROUTES` in `web/scripts/route-smoke.mjs`, imported by `a11y-scan.mjs` so the two gates cannot drift — in **both shipped locales** (en + zh-CN). Last master run (`922a2946`): `[a11y] clean — 41 routes × 2 locales scanned, 0 serious/critical violations`. This row cites the run instead of restating a count, because the inventory grows with the app. |
| FormControl label wiring (form fields)   | **gated**                    | `web/scripts/check-form-control.mjs` (chained into `bun run lint`): 140 `<FormControl>` sites classified, **0 unclassified** (the gate fails on a shape it cannot read rather than skipping it), and all 140 reach a DOM node carrying the injected `id` — 130 `ui/**` primitives + 10 forwarding composites. **0 registered exceptions**: the five composite controls (`EndpointsEditor` / `ScheduleEditor` / `ModelPolicyEditor` / `SiteScopePicker` / `CredentialRefPicker`) now carry the injected id on a `role="group"` root named by `aria-labelledby` (#1300, closed). |
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

**Current**: close buttons and icon-only chrome use the shared `--ring` recipe (`focus-visible:ring-3 focus-visible:ring-ring/50`); a single global focus-ring rule for every page-level action grid is residual (§7).

### 2.3 Keyboard traps

| Surface              | Expected                                                    | Notes                              |
| -------------------- | ----------------------------------------------------------- | ---------------------------------- |
| Search modal         | Esc exits; Tab cycles within modal                          | **Pass** — Base UI `Dialog` built-in focus trap on the panel |
| Centered modal       | Esc dismisses (Base UI `Dialog` default); close button always named | **Pass** — trap + dialog name      |
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
| Mobile hamburger / sidebar collapse | `切换侧边栏` (`Toggle Sidebar` key) — one static name for both directions | `web/src/components/ui/sidebar.tsx` (`SidebarTrigger`, mounted by `app-header.tsx`) |
| Language toggle          | `语言` (`common.language`); menu options are bilingual (`common.languageName.*`) | `web/src/components/layout/components/interface-controls.tsx` |
| Search trigger           | `搜索…` (`search.trigger`); the `Ctrl+K` hint is a visual `<Kbd>` sibling, not part of the name | `web/src/components/layout/components/app-header.tsx` |
| Theme menu trigger       | `切换主题` (`theme.toggle`) — static name | `web/src/components/layout/components/interface-controls.tsx` |
| User menu trigger        | `用户菜单` (`userMenu.trigger`)        | `web/src/components/layout/components/user-menu.tsx`   |
| Sidebar item (collapsed) | item label                           | `web/src/components/layout/components/app-sidebar.tsx` |
| Mobile drawer close      | `关闭` (`common.close`)              | `web/src/components/ui/sheet.tsx`                      |
| Modal close (×)          | `关闭` (`common.close`)              | `web/src/components/ui/dialog.tsx`                     |
| Search modal close       | `关闭` (`common.close`, via the dialog close) | `web/src/components/ui/command.tsx`           |

> 2026-09-13 correction: the header ships a `UserMenu` (version / About /
> documentation / sign-out), named by `userMenu.trigger` — it is in the table
> above. An earlier note here claimed no account menu existed because auth is
> token-based; that was wrong (the menu is not avatar-shaped, but it exists).

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

Ratios recomputed 2026-09-13 for the redesigned metapi palette (WCAG 2.x relative luminance over the shipped `web/src/styles/theme.css` OKLCH values; token map in `DESIGN.md` §2). Every constrained lightness is design-time-solved against its floor and pinned by `web/src/styles/__tests__/contrast-gate.test.ts`, which re-verifies every tracked pair — all 10 presets × both modes — on each run, so the numbers below are documentation, not the enforcement point.

### 4.1 Light theme

| Pair                                                                  | Ratio   | WCAG AA body (4.5:1) | Notes                                                                                                                                                  |
| --------------------------------------------------------------------- | ------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--foreground` on `--card` / `--background`                           | 19.7:1  | Pass                 | Titles, primary values                                                                                                                                 |
| `--muted-foreground` on `--card`                                      | 7.3:1   | Pass                 | Labels / secondary text                                                                                                                                |
| `--secondary-foreground` on `--secondary`                             | 12.8:1  | Pass                 | Nested wells                                                                                                                                           |
| `--primary-foreground` on `--primary`                                 | 4.5:1   | Pass                 | Light-theme CTA — white on the brand indigo; lightness is solved at the AA floor (maximum vibrancy that still passes), and the contrast gate holds it there                                                          |
| `--destructive-foreground` on `--destructive`                         | 4.6:1   | Pass                 | Errors, deletes — white on solved red                                                                                                                                                                     |
| Soft-badge text (`*-soft-fg` inks) on `/10` fills                     | ≥ 5.4:1 | Pass                 | 12px soft badges — every `*-soft-fg` is a standalone ink solved against the tint composited over the most-tinted bridge card (measured 5.48–5.76 across all 10 presets) |

### 4.2 Dark theme

| Pair                                      | Ratio  | WCAG AA body (4.5:1) | Notes                   |
| ----------------------------------------- | ------ | -------------------- | ----------------------- |
| `--foreground` on `--card`                | 14.2:1 | Pass                 | Titles, primary values  |
| `--foreground` on `--background`          | 15.7:1 | Pass                 | Page canvas             |
| `--muted-foreground` on `--card`          | 7.6:1  | Pass                 | Labels / secondary text |
| `--secondary-foreground` on `--secondary` | 10.8:1 | Pass                 | Nested wells            |
| `--primary-foreground` on `--primary`     | 6.8:1  | Pass                 | CTA — dark ink on the lifted pastel primary |
| `--destructive-foreground` on `--destructive` | 4.5:1 | Pass                | White on solved red; attention-bell critical badge |
| Soft-badge text (`*-soft-fg` inks) on `/10` fills | ≥ 5.1:1 | Pass           | Dark-mode warning/success reuse the base tone; info/destructive carry solved lifted inks (measured 5.14–6.85 across all 10 presets) |

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
| Sidebar             | Expanded 232px (`14.5rem`) default; collapsible to 44px (`2.75rem`) icon rail | Pass      |
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
| Page heading structure                 | One `h1` per page — the page header owns it; card titles are `h2` (no in-page breadcrumb)                                       | **Pass** (2026-08-12, Playwright: 1 h1 on maintenance / danger-zone / import-export); ownership re-verified in code 2026-09-12 — the `h1` is the page header's (`settings-page.tsx`, pinned by `settings/__tests__/settings-page-header.test.tsx`) and `components/common/section-card.tsx` renders the `h2`. There is no in-page breadcrumb (DESIGN.md §4.2: the sidebar tree is the only navigation surface). |

---

## 7. Known limitations

This section lists open residuals only. Closure history lives in the root [`CHANGELOG.md`](../../../CHANGELOG.md). Open work is committed through a scoped GitHub issue.

1. **Charts keyboard series access** — recharts renders series as non-focusable SVG; assistive tech relies on the text axes, legends, and rich text tooltips (balance/cost, accounts, calls, tokens, share) that already carry the data. Non-color status encoding (text labels on availability buckets, attention badges) is in place; no color-only status.
2. **Global focus-ring utility** — chrome controls share the `--ring` recipe (`focus-visible:ring-3 focus-visible:ring-ring/50`); a single shared rule for every page-level action grid is not yet in place.
3. **Hex hygiene** — no new brand hex is allowed in pages (see [`DESIGN.md`](./DESIGN.md) §1 Principles). Existing brand assets and other justified exceptions are reviewed when their owning surface changes; this is not a standalone sweep.

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
- Do not create a permanent “sweep” project. Promote a verified defect to a GitHub issue with a concrete owner and acceptance criterion.
