# Metapi Design System

**Product**: Metapi admin
**Scope**: Enterprise ops control plane (sites, accounts, tokens, routes, monitors, logs)
**Visual language**: GCP cloud console density + frosted glass shell + Apple detail
**Source of truth**: this document + `web/src/styles/theme.css` + `web/src/styles/theme-presets.css` + `web/src/lib/theme-customization.ts` + `web/src/components/ui/**`
**Last updated**: 2026-08-16

---

## 1. Brand

| Attribute       | Decision                                                                                                                                                                                                                                                                                                              |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Product voice   | Professional, dense, high-signal ops UI                                                                                                                                                                                                                                                                               |
| Audience        | Operators managing multi-site API gateways, keys, and routing                                                                                                                                                                                                                                                         |
| Personality     | Calm GCP control room with Apple-grade materials — not consumer marketing                                                                                                                                                                                                                                             |
| Density default | Comfortable-dense (admin tables + KPI cards coexist); density axis `data-theme-scale`                                                                                                                                                                                                                                 |
| Brand color     | **GCP Blue family** — light primary `oklch(0.692 0.141 243.716)`, dark primary `oklch(0.54 0.142 248.516)` (see §2.3; exact values live in `theme.css`, never hard-code hex)                                                                                                                                          |
| Logo mark       | Transparent solid-color badge `web/public/logo.svg` — rounded-square `#3b5bdb` field with white **π** glyph (real U+03C0, serif fallback, not hand-drawn strokes); `favicon.svg` = standalone solid blue π for small sizes; both served from the embedded SPA root (`router.go` root-file whitelist, `image/svg+xml`) |
| Fonts           | **Public Sans + Lora** (locally embedded via `@fontsource-variable`) — no Google Fonts CDN                                                                                                                                                                                                                            |
| High-res        | Content layout axis `data-theme-content-layout` (`full`/`centered`); centered clamps to `--max-content-width` (1280px) at ≥1280px viewport; utilities `max-w-container` (1280px) / `max-w-container-lg` (1536px)                                                                                                      |

**Principles**

1. **Signal over decoration** — every color/weight change means status, severity, or hierarchy.
2. **Token-first** — no new hard-coded hex in pages; use OKLCH CSS custom properties.
3. **Dual theme parity** — light and dark share the same semantic token names (`.dark` class on `<html>`).
4. **Dense but breathing** — small body text (default `--text-sm` ≈ 0.78rem), clear table rhythm, calm page titles (weight 400).
5. **One glass system** — shell/modal/dropdown only; never blur table rows.
6. **Progressive adoption** — primitives in `web/src/components/ui/**`; migrate pages gradually.
7. **Console, not marketing** — pill nav, tabular nums, restrained card hover (no lift).
8. **No gradients** — backgrounds, masks, charts, swatches, fallback avatars, and brand assets use solid colors only.

---

## 2. Color tokens

All values live in `web/src/styles/theme.css` under `:root` (light) and `.dark` (dark); presets override them per `data-theme-preset` in `theme-presets.css`.

### 2.1 Theme architecture

| Layer        | Mechanism                                                                                                                                                                         |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Theme mode   | `<html class="dark                                                                                                                                                                | light">`(plus`data-theme`attr for compat) set by`ThemeProvider`and the FOUC bootstrap in`web/index.html`; cookie `vite-ui-theme`(1y), falls back to`prefers-color-scheme` |
| Preset       | `<body data-theme-preset>` — 10 shipped presets: default, anthropic, simple-large, underground, rose-garden, lake-view, sunset-glow, forest-whisper, ocean-breeze, lavender-dream |
| Font axis    | `<body data-theme-font="sans                                                                                                                                                      | serif">`— swaps`--font-body`                                                                                                                                              |
| Radius axis  | `<body data-theme-radius="none                                                                                                                                                    | sm                                                                                                                                                                        | md                                      | lg  | xl">`— overrides`--radius` |
| Density axis | `<body data-theme-scale="sm                                                                                                                                                       | lg                                                                                                                                                                        | xl">`— rescales`--text-*`and`--spacing` |
| Content axis | `<body data-theme-content-layout="full                                                                                                                                            | centered">`                                                                                                                                                               |

Constants (allowed values, defaults, cookies `theme_preset` / `theme_font` / `theme_radius` / `theme_scale` / `theme_content_layout`): `web/src/lib/theme-customization.ts`.

### 2.2 Core semantic surfaces

| Token                     | Light                                 | Dark                                    | Usage                                             |
| ------------------------- | ------------------------------------- | --------------------------------------- | ------------------------------------------------- |
| `--background`            | `oklch(1 0 0)`                        | `oklch(0.235 0 0)`                      | App canvas                                        |
| `--foreground`            | `oklch(0.145 0 0)`                    | `oklch(0.965 0 0)`                      | Primary text                                      |
| `--card`                  | `oklch(1 0 0)`                        | `oklch(0.285 0 0)`                      | Cards, tables, drawers                            |
| `--popover`               | `oklch(1 0 0)`                        | `oklch(0.305 0 0)`                      | Popovers, menus, dropdowns                        |
| `--secondary` / `--muted` | `oklch(0.95 0 0)` / `oklch(0.97 0 0)` | `oklch(0.335 0 0)` / `oklch(0.305 0 0)` | Nested wells, subdued fills                       |
| `--muted-foreground`      | `oklch(0.49 0 0)`                     | `oklch(0.78 0 0)`                       | Meta/secondary text                               |
| `--border`                | `oklch(0.93 0 0)`                     | `oklch(1 0 0 / 10%)`                    | Hairlines                                         |
| `--input`                 | `oklch(0.93 0 0)`                     | `oklch(1 0 0 / 17%)`                    | Input borders                                     |
| `--ring`                  | `oklch(0.708 0.16 249.003)`           | `oklch(0.554 0.148 250.726)`            | Focus rings (`focus-visible:ring-3 ring-ring/50`) |
| `--overlay`               | `oklch(0 0 0 / 0.1)`                  | `oklch(0 0 0 / 0.32)`                   | Modal/sheet scrim                                 |

Each maps a Tailwind alias `--color-*` (e.g. `--color-background: var(--background)`) in the `@theme inline` block.

### 2.3 Brand / accent

| Token                                                  | Light                                                        | Dark                                                         | Usage                               |
| ------------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------ | ----------------------------------- |
| `--primary`                                            | `oklch(0.692 0.141 243.716)`                                 | `oklch(0.54 0.142 248.516)`                                  | Primary actions, active nav         |
| `--primary-foreground`                                 | `oklch(0.145 0 0)`                                           | `oklch(1 0 0)`                                               | Text on primary                     |
| `--accent`                                             | `color-mix(in oklch, var(--primary) 12%, var(--background))` | `color-mix(in oklch, var(--primary) 20%, var(--background))` | Soft primary fill / active chip     |
| `--neutral`                                            | `oklch(0.708 0 0)`                                           | `oklch(0.76 0 0)`                                            | Cool gray secondary                 |
| `--sidebar` / `--sidebar-primary` / `--sidebar-accent` | derived from background/primary                              | derived from background/primary                              | Sidebar canvas, brand, active tones |

Presets replace `--primary`/`--background` per `data-theme-preset` (e.g. Anthropic clay `oklch(0.57 0.15 38)` on cream `oklch(0.984 0.005 95)`).

### 2.4 Status semantics

| Token                                        | Light                                            | Dark                                             | Usage              |
| -------------------------------------------- | ------------------------------------------------ | ------------------------------------------------ | ------------------ |
| `--success` / `--success-foreground`         | `oklch(0.53 0.145 163.225)` / `oklch(0.985 0 0)` | `oklch(0.696 0.17 162.48)` / `oklch(0.145 0 0)`  | Healthy / active   |
| `--warning` / `--warning-foreground`         | `oklch(0.62 0.162 75.834)` / `oklch(0.145 0 0)`  | `oklch(0.769 0.188 70.08)` / `oklch(0.145 0 0)`  | Degraded / pending |
| `--destructive` / `--destructive-foreground` | `oklch(0.577 0.245 27.325)` / `oklch(0.985 0 0)` | `oklch(0.575 0.19 25)` / `oklch(0.985 0 0)`     | Errors, deletes    |
| `--info` / `--info-foreground`               | `oklch(0.53 0.158 241.966)` / `oklch(0.985 0 0)` | `oklch(0.613 0.14 239.919)` / `oklch(0.145 0 0)` | Informational      |

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
| Radius           | `--radius: 1rem` (default) with `--radius-sm/md/lg/xl/2xl/3xl/4xl` derived; `data-theme-radius` overrides `--radius` (none 0 · sm 0.3 · md 0.5 · lg 0.75 · xl 1rem)                                                                                                                                                                                 | Controls/buttons `rounded-lg`; cards/sheets `rounded-xl`+                                                                                                                                        |
| Shadow           | Tailwind default `shadow-*` + custom `--shadow-card-hover` (`0 4px 12px …`)                                                                                                                                                                                                                                                                         | Hover elevation on cards (`[data-card-hover]`); no lift on plain rows                                                                                                                            |
| Motion           | `tw-animate-css` utilities (`animate-in/out`, `fade-in-*`, `zoom-in-*`) + keyframes in `styles/index.css` (table row stagger, landing, terminal demo)                                                                                                                                                                                               | Calm; every animation guarded by `prefers-reduced-motion`                                                                                                                                        |
| Type             | `--font-sans` Public Sans Variable + CJK fallbacks (Noto Sans SC/TC/JP/KR, PingFang, Microsoft YaHei) · `--font-serif` Lora Variable + CJK serif fallbacks · `--font-mono` Cascadia/SFMono/Consolas · `--font-body` active face                                                                                                                     | `data-theme-font` swaps the body face; density axis rescales `--text-xs…3xl`; visible axis tick labels and body text minimum 10px — 9px only allowed for decorative labels with a `title`/`aria-label` fallback |
| Page title scale | Landing/hub pages (dashboard `Overview`, `About`, `Settings` overview, `Sign in`): `text-2xl font-normal tracking-tight` (24px) · data/list pages (models, sites, oauth, accounts, check-in, proxy-logs, token-routes, model-tester, settings sub-pages): `text-lg font-normal` (18px) · settings section cards own a single `text-base font-medium` h2 inside the card | Page h1s weight 400 ("calm titles"); settings section-card h2s weight 500 (matches `CardTitle` default); exactly one h1 per page (a11y); no third page-title variant without updating this table |

| Layout | `data-theme-content-layout="full|centered"`; centered clamps `[data-slot='sidebar-inset'] > *` to `--max-content-width` (1280px) at ≥1280px; utilities `max-w-container` 1280 / `max-w-container-lg` 1536 | Hi-res: `full` uses available width; `centered` keeps a comfortable reading width |

---

## 4. Components

Primitive ownership map: [`components.md`](./components.md). Component props and variants are defined by the implementation in `web/src/components/ui/**`, not duplicated in documentation.

State-management rules for URL-synced tables and filters (single URL owner, stable callbacks, one-transaction updates): [`state-stability.md`](./state-stability.md).

| Layer            | Prefix / classes                    | Where                                            |
| ---------------- | ----------------------------------- | ------------------------------------------------ |
| Base UI (shadcn) | `ui-*` components (data-slot attrs) | `web/src/components/ui/**`                       |
| Shell layout     | `app-header` / `app-sidebar`        | `web/src/components/layout/**`                   |
| Theme tokens     | OKLCH CSS variables                 | `web/src/styles/theme.css` + `theme-presets.css` |

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

---

## 5. Visual acceptance

1. `cd web && bun run test` — vitest unit/component suites
2. `cd web && bun run typecheck` — TS gate
3. `cd web && bun run lint` — oxlint
4. `cd web && bun run build` — production bundle gate (`build:web`)
5. `cd web && bun run a11y:scan` — axe-core serious/critical gate (needs the dev server; see `web/scripts/a11y-scan.mjs`). Also enforced in CI: the `a11y` job serves the real embedded SPA via the Go server (fresh sqlite runtime DB) and scans all 15 admin routes against it (`BASE_URL`-driven; `.github/workflows/main.yml`)
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

Visual change history lives in root [`CHANGELOG.md`](../../../CHANGELOG.md) and [`../log.md`](../log.md); this document states only current truth.
