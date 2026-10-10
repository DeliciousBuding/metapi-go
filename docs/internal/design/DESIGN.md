# Metapi Design System

**Product**: Metapi admin
**Scope**: Enterprise ops control plane (sites, accounts, tokens, routes, monitors, logs)
**Visual language**: GCP cloud console density + frosted glass shell + Apple detail
**Source of truth**: this document + `web/src/styles/theme.css` + `web/src/styles/theme-presets.css` + `web/src/lib/theme-customization.ts` + `web/src/components/ui/**`
**Last updated**: 2026-09-12

---

## 1. Brand

| Attribute       | Decision                                                                                                                                                                                                                                                                                                              |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Product voice   | Professional, dense, high-signal ops UI                                                                                                                                                                                                                                                                               |
| Audience        | Operators managing multi-site API gateways, keys, and routing                                                                                                                                                                                                                                                         |
| Personality     | Calm GCP control room with Apple-grade materials — not consumer marketing                                                                                                                                                                                                                                             |
| Density default | Comfortable-dense (admin tables + KPI cards coexist); density axis `data-theme-scale`                                                                                                                                                                                                                                 |
| Brand color     | **Indigo family** — light primary `oklch(0.565 0.19 262)` (white text), dark primary `oklch(0.71 0.155 262)` (dark ink); every constrained lightness is solved against its WCAG floor (see §2.3; exact values live in `theme.css`, never hard-code hex)                                                                                                                                          |
| Logo mark       | Transparent solid-color badge `web/public/logo.svg` — rounded-square `#3b5bdb` field with white **π** glyph (real U+03C0, serif fallback, not hand-drawn strokes); `favicon.svg` = standalone solid blue π for small sizes; both served from the embedded SPA root (`router.go` root-file whitelist, `image/svg+xml`) |
| Fonts           | **Inter + native CJK**, optional **Lora** (locally embedded via `@fontsource-variable`) — no Google Fonts CDN                                                                                                                                                                                                                            |
| High-res        | Content layout axis `data-theme-content-layout` (`full`/`centered`); full remains fluid until 1920px, then caps the content scan width at 1920px; centered clamps to `--max-content-width` (1280px) at ≥1280px. The sidebar and header remain viewport-wide. Utilities `max-w-container` (1280px) / `max-w-container-lg` (1536px) serve individual pages. |

**Primary workspace**: desktop administration. Judge hierarchy, density and
multi-column composition at 1440–1920px first. Mobile adapts the same tasks
without letting narrow-screen compromises dictate the desktop layout.
Metric cards keep title, primary value, supporting text and detail cells in
separate visual tiers; long supporting text must not compete beside the number.
Desktop table rows start at 48px and grow for multiline content; narrow layouts
retain their existing touch spacing. Desktop lists with four or fewer loaded rows
hug their content rather than stretching a mostly empty bordered table; longer
lists retain the sticky header. The [operator overview](dashboard-operations.md)
prioritizes request outcomes, actionable risks and upstream/model usage; resource
counts and maintenance are secondary. Metric details use compact divided cells
rather than nested padded cards. Increase information density by removing repeated spacing,
not by shrinking body text.

Proxy logs show attempt timing, input/output token counts, estimated USD cost,
and stream mode in the default view. API keys are not labeled as token usage.
Account, client, key and retry columns remain optional; saved column preferences
are preserved. Missing usage, cost and mode stay unknown rather than becoming
zero or non-stream. Billing details present billable input, output, cache read
and cache creation with their individual costs; the original billing object is
available in a disclosure, including legacy objects without that breakdown.

**Principles**

1. **Signal over decoration** — every color/weight change means status, severity, or hierarchy.
2. **Token-first** — no new hard-coded hex in pages; use OKLCH CSS custom properties.
3. **Dual theme parity** — light and dark share the same semantic token names (`.dark` class on `<html>`).
4. **Dense but breathing** — 14px body text by default, component-owned secondary text, clear table rhythm, and semibold page titles. The compact density axis scales text separately.
   Mobile card labels use `text-xs`, values `text-sm`; detail-sheet labels use `text-xs`. Keep list/header text compact, but show the complete value in the detail layer (wrapping URLs and diagnostic reasons rather than silently ellipsizing them). All sizes follow the density axis.
5. **One glass system** — shell/modal/dropdown only; never blur table rows.
6. **One primitive set** — UI starts from `web/src/components/ui/**` and nothing hand-rolls a parallel control. This used to be a migration in progress; it is now gated — `web/scripts/check-form-control.mjs` classifies every `<FormControl>` site and fails on a native control or on a shape it cannot read.
7. **Console, not marketing** — pill nav, tabular nums, restrained card hover (no lift).
8. **No gradients** — backgrounds, masks, charts, swatches, fallback avatars, and brand assets use solid colors only.

---

### Shared page and task composition

The shared primitives also own how a task is presented; pages should not
rebuild these arrangements with local spacing and raw data editors:

- `components/common/page-header.tsx`: standard list/settings/tool page title,
  description and wrapping actions. Overview pages retain their overview
  hierarchy. Page containers continue to own scrolling.
- `components/data-table/toolbar/toolbar.tsx`: separate query and view/action
  groups, with expanded filters in their own region. The desktop and mobile
  empty states share `core/empty-state.tsx`; page-owned filters participate in
  the same reset action as table filters.
- `features/settings/components/settings-subsection.tsx`: a settings group
  with title, optional description/icon/actions, and flat or panel treatment.
  Notifications and backup tasks use this same composition. Save/reset status
  lives in `settings-form-actions.tsx`, including a polite live announcement
  and responsive action placement.
- `components/common/string-map-editor.tsx`: string mappings use labeled
  key/value rows by default. JSON is an explicit advanced view, retains the
  original text, and remains available for legacy object values that cannot
  be represented as rows. A mode switch must not discard data.

## 2. Color tokens

Independent upstreams use the same brand identities as site presets. Creation
starts with New API, domestic platforms and Coding Plan presets; protocol names
remain separate from provider names. The detail sheet uses four short tabs:
connection, credentials, models and routes. Models expand on demand, and batch
entry uses one upstream model name per line. Access always selects a credential
and executable protocols explicitly. Capabilities are grouped as conversation,
retrieval, images, audio and video; preset cards summarize these groups, while
endpoint details expand individually for editing. Only the four conversation
protocols offer conversion ordering; media permissions remain exact capabilities.
Refetch updates untouched fields while
preserving edits. Deleting an edited entity first confirms draft loss; leaf
deletion retains undo, and dependent deletion reviews the server's current impact.

### Shared page and task composition

The shared primitives also own how a task is presented; pages should not
rebuild these arrangements with local spacing and raw data editors:

- `components/common/page-header.tsx`: standard list/settings/tool page title,
  description and wrapping actions. Overview pages retain their overview
  hierarchy. Page containers continue to own scrolling.
- `components/data-table/toolbar/toolbar.tsx`: separate query and view/action
  groups, with expanded filters in their own region. The desktop and mobile
  empty states share `core/empty-state.tsx`; page-owned filters participate in
  the same reset action as table filters.
- `features/settings/components/settings-subsection.tsx`: a settings group
  with title, optional description/icon/actions, and flat or panel treatment.
  Notifications and backup tasks use this same composition. Save/reset status
  lives in `settings-form-actions.tsx`, including a polite live announcement
  and responsive action placement.
- `components/common/string-map-editor.tsx`: string mappings use labeled
  key/value rows by default. JSON is an explicit advanced view, retains the
  original text, and remains available for legacy object values that cannot
  be represented as rows. A mode switch must not discard data.
- Backup import starts with file selection, then a human-readable plan and
  confirmation. JSON paste is collapsed by default. Counts use labeled summary
  cells; compatibility failures precede details, and omitted/adapted data
  requires acknowledgement. Raw field names are not primary UI labels.

All values live in `web/src/styles/theme.css` under `:root` (light) and `.dark` (dark); presets override them per `data-theme-preset` in `theme-presets.css`.

### 2.1 Theme architecture

| Layer | Mechanism |
| ----- | --------- |
| Theme mode   | `<html>` carries `class="dark"` or `class="light"` plus a `data-theme` attribute (compat), set by `ThemeProvider` and the FOUC bootstrap in `web/index.html`; persisted in the `vite-ui-theme` cookie (1y) and falling back to `prefers-color-scheme` |
| Preset       | `<body data-theme-preset>` — 10 shipped presets: default, graphite, cobalt, lagoon, kelp, moss, ochre, ember, berry, plum |
| Font axis    | `<body data-theme-font>` — `sans` or `serif` (the `default` setting resolves to `sans`, see `public/theme-init.js`); swaps `--font-body` |
| Radius axis  | `<body data-theme-radius>` — `default`, `none`, `sm`, `md`, `lg`, `xl`; overrides `--radius` |
| Density axis | `<body data-theme-scale>` — `default`, `sm`, `lg`, `xl`; rescales `--text-*` and `--spacing` |
| Content axis | `<body data-theme-content-layout>` — `full` or `centered` |

Constants (allowed values, defaults, cookies `theme_preset` / `theme_font` / `theme_radius` / `theme_scale` / `theme_content_layout`): `web/src/lib/theme-customization.ts`.

### 2.2 Core semantic surfaces

| Token                     | Light                                 | Dark                                    | Usage                                             |
| ------------------------- | ------------------------------------- | --------------------------------------- | ------------------------------------------------- |
| `--background`            | `oklch(1 0 0)`                        | `oklch(0.205 0.006 262)`                | App canvas                                        |
| `--foreground`            | `oklch(0.15 0 0)`                     | `oklch(0.955 0.004 262)`                | Primary text                                      |
| `--card`                  | `oklch(1 0 0)`                        | `oklch(0.245 0.007 262)`                | Cards, tables, drawers                            |
| `--popover`               | `oklch(1 0 0)`                        | `oklch(0.265 0.007 262)`                | Popovers, menus, dropdowns                        |
| `--secondary` / `--muted` | `oklch(0.955 0.004 262)` / `oklch(0.966 0.002 262)` | `oklch(0.3 0.008 262)` / `oklch(0.275 0.007 262)` | Nested wells, subdued fills                       |
| `--muted-foreground`      | `oklch(0.455 0.008 262)`              | `oklch(0.76 0.008 262)`                 | Meta/secondary text                               |
| `--border`                | `oklch(0.918 0.003 262)`              | `oklch(1 0 0 / 11%)`                    | Hairlines                                         |
| `--input`                 | `oklch(0.918 0.003 262)`              | `oklch(1 0 0 / 18%)`                    | Input borders                                     |
| `--ring`                  | `oklch(0.6 0.15 262)`                 | `oklch(0.64 0.13 262)`                  | Focus rings (`focus-visible:ring-3 ring-ring/50`) |
| `--overlay`               | `oklch(0 0 0 / 0.12)`                 | `oklch(0 0 0 / 0.34)`                   | Modal/sheet scrim                                 |

Each maps a Tailwind alias `--color-*` (e.g. `--color-background: var(--background)`) in the `@theme inline` block.

### 2.3 Brand / accent

| Token                                                  | Light                                                        | Dark                                                         | Usage                               |
| ------------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------ | ----------------------------------- |
| `--primary`                                            | `oklch(0.565 0.19 262)`                                      | `oklch(0.71 0.155 262)`                                      | Primary actions, active nav         |
| `--primary-foreground`                                 | `oklch(0.985 0 0)`                                           | `oklch(0.21 0.03 262)`                                       | Text on primary                     |
| `--accent`                                             | `color-mix(in oklch, var(--primary) 12%, var(--background))` | `color-mix(in oklch, var(--primary) 20%, var(--background))` | Soft primary fill / active chip     |
| `--neutral`                                            | `oklch(0.556 0 0)`                                           | `oklch(0.7 0.004 262)`                                       | Cool gray secondary                 |
| `--sidebar` / `--sidebar-primary` / `--sidebar-accent` | derived from background/primary                              | derived from background/primary                              | Sidebar canvas, brand, active tones |

Presets replace `--primary` and the derived accent/ring/chart slots per `data-theme-preset` (e.g. ember `oklch(0.575 0.17 40)` with its split-complementary teal secondary); `graphite` carries a full achromatic palette.

### 2.4 Status semantics

| Token                                        | Light                                            | Dark                                             | Usage              |
| -------------------------------------------- | ------------------------------------------------ | ------------------------------------------------ | ------------------ |
| `--success` / `--success-foreground`         | `oklch(0.54 0.14 152)` / `oklch(0.985 0 0)`      | `oklch(0.71 0.14 152)` / `oklch(0.19 0.03 152)`  | Healthy / active   |
| `--warning` / `--warning-foreground`         | `oklch(0.68 0.15 82)` / `oklch(0.24 0.04 82)`    | `oklch(0.79 0.14 82)` / `oklch(0.2 0.035 82)`    | Degraded / pending |
| `--destructive` / `--destructive-foreground` | `oklch(0.58 0.21 24)` / `oklch(0.985 0 0)`       | `oklch(0.58 0.19 24)` / `oklch(0.985 0 0)`       | Errors, deletes    |
| `--info` / `--info-foreground`               | `oklch(0.54 0.12 222)` / `oklch(0.985 0 0)`      | `oklch(0.7 0.11 222)` / `oklch(0.19 0.025 222)`  | Informational      |

Badge pattern: solid on soft fill (e.g. `bg-success/10 text-success`); each status also maps a `--color-*` Tailwind alias.

### 2.5 Charts

`--chart-1…5` in both themes drive recharts series (SVG resolves the CSS `var()` palette directly); dark reuses the same five hues at lighter lightness. Non-color status encoding is required for availability buckets (see a11y checklist).

### 2.6 Glass material

No dedicated `--glass-*` variables. Glass is a Tailwind recipe:

| Surface                | Recipe                                                                          |
| ---------------------- | ------------------------------------------------------------------------------- |
| Topbar / sticky chrome | `bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg` |
| Modal / sheet scrim    | `bg-overlay supports-backdrop-filter:backdrop-blur-xs`                          |
| Floating toolbars      | `bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg` |

Fallback: `supports-[backdrop-filter]` gates translucency so browsers without `backdrop-filter` keep a solid surface; `prefers-reduced-transparency` reduces to solid elevated surface.

### 2.7 Focus

`--ring` (per theme) + shadcn recipe `focus-visible:ring-3 focus-visible:ring-ring/50`; global base `outline-ring/50` in `styles/index.css`. Never remove the outline without a visible replacement ring.

---

## 3. Spacing, radius, elevation, motion, type, layout

| Family           | Mechanism                                                                                                                                                                                                                                                                                                                                           | Notes                                                                                                                                                                                            |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Spacing          | Tailwind 4 scale — base `--spacing` (default 0.25rem; `data-theme-scale` overrides 0.225 / 0.28 / 0.3rem)                                                                                                                                                                                                                                           | `gap-*`, `p-*`, `m-*`, `space-y-*`; no custom `--space-*`                                                                                                                                        |
| Radius           | `--radius: 0.625rem` (10px default) with `--radius-sm/md/lg/xl/2xl/3xl/4xl` derived; `data-theme-radius` overrides `--radius` (none 0 · sm 0.3 · md 0.5 · lg 0.75 · xl 1rem)                                                                                                                                                                                 | Controls/buttons `rounded-lg`; cards/sheets `rounded-xl`+                                                                                                                                        |
| Shadow           | Tailwind default `shadow-*` + custom `--shadow-card-hover` (`0 4px 12px …`)                                                                                                                                                                                                                                                                         | Hover elevation on cards (`[data-card-hover]`); no lift on plain rows                                                                                                                            |
| Motion           | `tw-animate-css` utilities (`animate-in/out`, `fade-in-*`, `zoom-in-*`) + the keyframes actually defined in `styles/index.css` (`tableRowEnter`, `sidebarViewEnter`, `slideDown`/`slideUp`, skeleton `shimmer`)                                                                                                                                                                                               | Calm; every animation guarded by `prefers-reduced-motion`                                                                                                                                        |
| Type             | `--font-sans` Inter Variable (Latin) + native CJK fallbacks (PingFang / Microsoft YaHei UI / Noto Sans CJK) · `--font-serif` Lora Variable + CJK serif fallbacks (native CJK serif fallback) · `--font-mono` Cascadia/SFMono/Consolas · `--font-body` active face · `:root[lang\|='zh']` zeroes `--tracking-tight` and the editorial tracking tokens (negative tracking is a Latin display convention; ideographs keep their side bearing)                                                                                                                     | `data-theme-font` swaps the body face; density axis rescales `--text-2xs…3xl` (sub-xs caption tokens registered in theme.css so captions ride the axis instead of freezing at px literals); visible axis tick labels and body text minimum 10px — 9px only allowed for decorative labels with a `title`/`aria-label` fallback |
| Page title scale | Landing/hub pages: `page-title-overview` (30px default); data/list pages: `page-title` (24px default). Both use semibold, tight leading and balanced wrapping | Exactly one h1 per page; Chinese tracking stays neutral; serif axis retains medium weight; title line-height follows density scaling |
| Layout | `data-theme-content-layout` remains `full` or `centered`; full caps content at 1920px above ultrawide viewports, centered caps at 1280px from 1280px viewport width; utilities `max-w-container` 1280 / `max-w-container-lg` 1536 | Header and sidebar remain viewport-wide; only the inset content is constrained |

### 3.1 Reading hierarchy and controls

The common reading roles are page title (24px; overview 30px), section/card title (`text-base` semibold), KPI (`text-xl` to `text-3xl` monospace semibold with tabular numerals and `leading-tight`), table header (`text-xs` semibold), row body (`text-sm`), and field label (`text-xs` muted). Do not use opacity on tiny sidebar section labels: their foreground must stay readable. The active nav link must forward both `data-active` and `aria-current=page` through its Link bridge, so visible selection and the accessibility announcement agree. These are component-owned roles, not a second parallel set of typography tokens.

Model names use the shared `ModelPill` identity: a vendored brand glyph beside the full model identifier, with truncation and a full-name tooltip. Use its pill variant in dense records and its inline variant in selectors, rankings and headings where another container already supplies the surface. Unknown models keep a deterministic lettermark; never borrow an unrelated vendor logo. Decorative glyphs are hidden from assistive technology so a selectable model keeps its exact accessible name. Aggregates such as Other are not model identities. Status badges remain separate from brand identity.

Proxy-log failure reasons use the shared keyboard-operable two-line disclosure. On mobile, status and failure copy occupy a full-width labeled field below the timestamp. Row Enter/Space shortcuts ignore nested interactive controls. Timing is a two-row label/value display shared by the list and detail sheet: response-header TTFB and complete attempt duration remain distinct, with missing measurements shown as unknown. Account editing uses semibold field labels and 24px form spacing with the shared form-control primitives.

Tables inherit the 14px body scale and tabular numerals. Cells and descendants must not be force-sized by the table primitive: explicit metadata, badge and secondary-label sizes belong to their components. Default control height is 36px (small 32px, large 40px); the default radius token is 10px. User-selected density, radius and font axes remain independent.

Detail sheets keep 12px medium labels above 14px normal-leading values. Plain text longer than 96 characters or spanning more than two source lines spans the detail grid, shows two lines at rest and exposes a keyboard-focusable expand/collapse control; the full text remains in the DOM for selection and assistive technology. Rich values (such as safe links) may use the same disclosure while retaining their link semantics. Diagnostic copy uses the same disclosure, not a hover-only tooltip. Mobile card titles use 14px semibold; labels stay 12px medium and values 14px normal-leading, while tertiary metadata may use the density-scaled 2xs role. Long values wrap, and long titles may occupy up to two lines rather than being silently cut to one.

Route editing presents matching and account selection first; advanced display/routing fields stay in a disclosure that preserves drafts. Invalid advanced fields must reveal and use the shared form validation focus. Filtering accounts limits bulk selection to visible matches without clearing hidden selections. Rebuild success uses a compact summary with optional metrics; partial failures, observation failures and retry actions remain visible.

Channels keep account-derived channels as the default view. Imported upstreams have a separate list and a detail sheet with connection, credential and route tabs; an installation containing only imported upstreams opens that list directly. Provider identity uses a logo capsule, model identity uses `ModelPill`, and protocol names describe the connection rather than masquerading as providers. Tabs retain drafts and the sheet uses the shared dirty-close guard. Request settings load only after an explicit edit action and never enter Query caches; credential replacement clears inputs only after success. Eligibility, disabled dependencies and temporary cooldown are distinct states.

### 3.2 Font roles and rendering

Inter is locally bundled for Latin UI text. Chinese follows the platform UI face (PingFang on macOS, Microsoft YaHei UI on Windows, Noto Sans CJK on Linux). This intentionally prioritizes native CJK rendering over identical glyphs on every platform; screenshot baselines must run in a fixed font environment. Public Sans and the bundled Noto Sans SC slices are no longer shipped.

Model identifiers use the standard sans-serif UI stack at 14px medium weight, including inside capsules and selectors; do not style model names as code. The medium weight distinguishes identities from secondary counts without making long identifiers look bold. Provider headings in the picker use semibold with quiet separators; overview rows keep model names at 14px and counts at 12px. Actionable attention text is 14px, not caption-sized. KPI figures retain the shared monospace stack. Model identity rows align a 16px icon box with a 20px text line, use middle vertical alignment in surrounding inline text, and retain the complete identifier in a tooltip. Body, title, numeric and code roles remain distinct; do not stretch glyphs or compensate with transforms.

Validate actual rendered font names and model icon/text box centers in the browser, then inspect light/dark desktop screenshots. A font-family declaration alone does not prove which CJK face rendered.

---

## 4. Components

Primitive ownership map: [`components.md`](./components.md). Component props and variants are defined by the implementation in `web/src/components/ui/**`, not duplicated in documentation.

State-management rules for URL-synced tables and filters (single URL owner, stable callbacks, one-transaction updates): [`state-stability.md`](./state-stability.md).

| Layer                     | Prefix / classes                    | Where                                            |
| ------------------------- | ----------------------------------- | ------------------------------------------------ |
| Base UI (shadcn)          | `ui-*` components (data-slot attrs) | `web/src/components/ui/**`                       |
| Cross-feature composition | section card / error / skeleton, query-error banner, confirm dialog, HTTP status badge + the shared soft-tone badge recipe | `web/src/components/common/**` |
| Table subsystem           | data-table barrel (`@/components/data-table`) | `web/src/components/data-table/**`     |
| Form plumbing             | dirty-close guard                   | `web/src/components/form/**`                    |
| Shell layout              | `app-header` / `app-sidebar`        | `web/src/components/layout/**`                   |
| Theme tokens              | OKLCH CSS variables                 | `web/src/styles/theme.css` + `theme-presets.css` |

New UI must start from shadcn Base UI primitives when possible. Import via `@/components/ui/*`.

### 4.1 Destructive-action tiers（破坏性操作四档）

Every destructive action maps to exactly one tier; never hand-roll a variant:

| 档 | 形态 | 适用 | 实现 |
|---|---|---|---|
| 直达 | 无确认 | 可逆切换（启用/停用、置顶、刷新） | 直接 mutation |
| 删除+undo | 无弹窗；行即消失 + 6s 可撤销 toast | 叶子实体单行删除（redirects、catalog sources、routes、downstream keys、account tokens） | `useUndoableDelete`（`@/lib/undoable-delete`） |
| 批量确认 | 计数确认弹窗 | 批量操作、清空、跨页应用 | `ConfirmDialog`（含 count 文案） |
| typed-confirm | 倒计时 + 输入确认词 | 不可逆/级联重操作（factory reset、站点/账号级联删除） | danger-zone 模式（倒计时 + 确认词） |

例外必须注释标注（例：OAuth 连接删除用 ConfirmDialog + 既有跨页乐观回滚 hook，不移交 undo helper）。

### 4.2 Settings page skeleton（设置页骨架）

- 单 h1 在 `SettingsPage` 页头，文案来自 section meta 的 i18n 键；侧栏树是唯一导航面（无页内面包屑或二级侧栏）。
- `SectionCard`（`components/common/section-card.tsx`，settings 各节与 `/downstream-keys` 页共用）**无 `actions` 时不渲染 CardHeader**——同一组 title/description i18n 键已在页头渲染，卡头再渲染就是逐字重复。带 header actions（测试/保存/批量按钮）的卡保留卡头（按钮需要宿主），但**单卡 section 传 `hideHeaderCopy`**：页头同题文案已在页面顶层，h2 重复无辨识价值；h2 只保留给多内容页内需区分的卡片。
- 危险/前置警告信息放卡内正文顶部 banner（`border-warning/40 bg-warning/5` + `TriangleAlert`），不藏在 description 里（参考：数据迁移节的 wipe/重启警告）。

---

## 5. Visual acceptance

1. `cd web && bun run test` — vitest unit/component suites
2. `cd web && bun run typecheck` — TS gate
3. `cd web && bun run lint` — oxlint
4. `cd web && bun run build` — production bundle gate (`build:web`)
5. `cd web && bun run a11y:scan` — axe-core serious/critical gate (needs the dev server; see `web/scripts/a11y-scan.mjs`). Also enforced in CI: the `a11y` job serves the real embedded SPA via the Go server (fresh sqlite runtime DB) and scans it in both shipped locales (`BASE_URL`-driven; `.github/workflows/main.yml`). The route list is `DESKTOP_ROUTES` in `web/scripts/route-smoke.mjs`, which `a11y-scan.mjs` imports so the two gates cannot drift — this document deliberately does not restate the count.
6. `cd web && bun run ui:smoke` — real-Chromium route/crash/mobile smoke gate (`web/scripts/route-smoke.mjs`); also enforced in CI in the `a11y` job against the shipped bundle
7. Manual score rubric (target ≥ 4/5 each):
   - Material (glass/solid hierarchy)
   - Brand calm (GCP blue, no neon)
   - Spacing rhythm
   - Card elevation / radius
   - Motion restraint
   - Dark parity

---

## 6. a11y non-negotiables

- Focus-visible rings via the `--ring` recipe (`focus-visible:ring-3 ring-ring/50`)
- Contrast on soft badges in both themes
- `prefers-reduced-transparency` / `prefers-reduced-motion`
- FOUC: cookie-first bootstrap in `web/index.html`; no white flash in dark

Checklist: [`a11y-checklist.md`](./a11y-checklist.md).

---

## 7. History

Visual change history lives in root [`CHANGELOG.md`](../../../CHANGELOG.md); this document states only current truth.
