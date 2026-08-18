# Metapi Accessibility & Responsive Checklist

**Product**: Metapi admin
**Scope**: accessibility checklist
**Related source of truth**: `docs/design/DESIGN.md`, `web/src/styles/theme.css`
**Last updated**: 2026-08-18
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

Ratios computed 2026-08-12 from the shipped `web/src/styles/theme.css` OKLCH values (WCAG 2.x relative luminance; token map in `DESIGN.md` §2). The light-theme primary CTA uses ink `--primary-foreground` on `--primary` (7.28:1, AAA) by design.

### 4.1 Light theme

| Pair                                                                  | Ratio   | WCAG AA body (4.5:1) | Notes                                                                                                                                                  |
| --------------------------------------------------------------------- | ------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--foreground` on `--card` / `--background`                           | 20.9:1  | Pass                 | Titles, primary values                                                                                                                                 |
| `--muted-foreground` on `--card`                                      | 6.3:1   | Pass                 | Labels / secondary text                                                                                                                                |
| `--secondary-foreground` on `--secondary`                             | 11.8:1  | Pass                 | Nested wells                                                                                                                                           |
| `--primary-foreground` on `--primary`                                 | 7.28:1  | Pass                 | Light-theme CTA — ink-on-brand (`--primary-foreground` = `oklch(0.145 0 0)` on the brand primary), AAA by design                                                          |
| White text on `--destructive`                                         | 4.8:1   | Pass                 | Errors, deletes                                                                                                                                        |
| Soft-badge text (`--success` / `--info` / `--warning`) on `/10` fills | ≥ 4.5:1 | Pass                 | 12px soft badges — light `--success`/`--info`/`--warning` lightness lowered (0.53 / 0.53 / 0.62) 2026-08-14; `*-soft-fg` tokens ship the readable tone |

### 4.2 Dark theme

| Pair                                      | Ratio  | WCAG AA body (4.5:1) | Notes                   |
| ----------------------------------------- | ------ | -------------------- | ----------------------- |
| `--foreground` on `--card`                | 13.0:1 | Pass                 | Titles, primary values  |
| `--foreground` on `--background`          | 15.1:1 | Pass                 | Page canvas             |
| `--muted-foreground` on `--card`          | 7.2:1  | Pass                 | Labels / secondary text |
| `--secondary-foreground` on `--secondary` | 8.9:1  | Pass                 | Nested wells            |
| White text on `--primary`                 | 5.0:1  | Pass                 | CTA                     |

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
| Sidebar       | Hidden; content via `MobileDrawer`                                  | Pass                     |
| Main padding  | Reduced; no clipped primary CTA                                     | Pass / page debt         |
| Tables        | Card/list alternative or horizontal scroll inside table region only | Partial (page-dependent) |
| Batch actions | `MobileBatchBar` / responsive batch bar                             | Partial                  |
| Filters       | `ResponsiveFilterPanel` → bottom/side sheet                         | Partial                  |
| Touch targets | ≥36–44px for chrome icons                                           | Pass topbar              |
| Safe areas    | Avoid fixed bars covering primary content                           | Residual on some pages   |

### 5.2 768px (tablet / breakpoint edge)

| Check            | Expected                                       | Status            |
| ---------------- | ---------------------------------------------- | ----------------- |
| Layout switch    | Mobile drawer path active at ≤768              | Pass              |
| Topbar density   | Tools remain usable without overlap            | Pass              |
| Modals           | Max-width constrained; close control reachable | Pass shared modal |
| Two-column forms | Stack via `ResponsiveFormGrid` where used      | Partial adoption  |

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
| `prefers-reduced-transparency: reduce` | Glass → solid elevated; strip backdrop blur                                                                                     | **Pass** — glass family + sidebar/topbar tokens solidify; shell/login/toast/overlay blur stripped in `web/src/styles/index.css` + shadcn Base UI glass surfaces |
| Dialog semantics                       | `role="dialog"` + `aria-modal` for blocking overlays                                                                            | Mobile drawer pass; SearchModal improved; not all legacy overlays                                                                                               |
| Live regions                           | Toasts/errors announced                                                                                                         | Residual                                                                                                                                                        |
| Language                               | `t()` for user-visible chrome strings; `aria-label` included in i18n attr list                                                  | Pass pattern                                                                                                                                                    |
| Page heading structure                 | One `h1` per settings section page — breadcrumb header (`Settings / subarea`) + section card owns the single `h1` + description | **Pass** (2026-08-12): `settings-page.tsx` + `settings-section-card.tsx`; verified via Playwright (1 h1 on maintenance / danger-zone / import-export)           |

---

## 7. Known limitations

This section lists open residuals only. Closure history lives in [`../log.md`](../log.md) and root [`../../CHANGELOG.md`](../../CHANGELOG.md). Open work is committed through [`../progress/MASTER.md`](../progress/MASTER.md) or a scoped issue.

1. **Charts keyboard series access** — recharts renders series as non-focusable SVG; assistive tech relies on the text axes, legends, and rich text tooltips (balance/cost, accounts, calls, tokens, share) that already carry the data. Non-color status encoding (text labels on availability buckets, attention badges) is in place; no color-only status.
2. **Global focus-ring utility** — chrome controls share the `--ring` recipe; a single shared rule for every page-level action grid is not yet in place (`.modal-close-button:focus-visible` uses a primary outline).
3. **Hex hygiene** — no new brand hex is allowed in pages (see [`DESIGN.md`](./DESIGN.md) §1 Principles). Existing brand assets and other justified exceptions are reviewed when their owning surface changes; this is not a standalone sweep.
4. **Preset contrast (audit-only)** — the default theme passes AA/AAA; user-chosen presets are explicit choices and out of scope. Underground/rose-garden/sunset-glow pass with white; forest-whisper, ocean-breeze, and lavender-dream remain below AA on white text — a preset-specific fix is deferred until a real user need appears.

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
