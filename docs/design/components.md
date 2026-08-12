# Design system components

> Last updated: 2026-08-11
> The legacy `ds-*` design system was replaced by **shadcn Base UI** components during the 2026-08 frontend rewrite (Bun + Rsbuild). New primitives live in `web/src/components/ui/**`; the inventory below documents the pre-rewrite system and is kept as a historical reference.
> Source of truth for primitives: `web/src/components/ui/**`
> Tokens: `web/src/styles/theme.css` + `docs/design/DESIGN.md`

## Purpose

shadcn Base UI primitives for the admin UI. Prefer these over ad-hoc Tailwind / inline styles for new surfaces. Colors and elevation come from OKLCH CSS variables (`var(--color-*)`, theme tokens in `web/src/styles/theme.css`).

## Entry points

| Path | Role |
|------|------|
| `web/src/components/ui/**` | shadcn Base UI primitives (button, dialog, table, …) |
| `web/src/components/layout/**` | Shell chrome (app-header, app-sidebar, nav groups) |
| `web/src/styles/index.css` | Tailwind 4 entry + global styles |

## Access (gallery)

The `/__design__` dev gallery and `DesignSystemGallery` page were removed with the Playwright visual suite (2026-08 rewrite). Theme toggles set `document.documentElement[data-theme]` to `light` or `dark`.

## Tokens used

Primitives consume the OKLCH semantic tokens from `web/src/styles/theme.css`
(surfaces `--background`/`--card`/`--popover`, ink `--foreground`/`--muted-foreground`,
status `--success`/`--warning`/`--destructive`/`--info`, focus `--ring`,
borders `--border`/`--input`, charts `--chart-1…5`) via their Tailwind `--color-*`
aliases. Glass is a recipe, not a token: `bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg`
for sticky chrome, `bg-overlay supports-backdrop-filter:backdrop-blur-xs` for modal/sheet scrims.
Spacing/radius/motion follow the Tailwind 4 scale plus the `data-theme-radius` / `data-theme-scale`
axes documented in `DESIGN.md` §2–3.

## Inventory

### Button

```tsx
import { Button } from '../design-system/index.js';

<Button variant="primary" size="md">Save</Button>
```

| Prop | Values | Default |
|------|--------|---------|
| `variant` | `primary` \| `secondary` \| `ghost` \| `danger` | `primary` |
| `size` | `sm` \| `md` | `md` |

Classes: `ds-btn`, `ds-btn--{variant}`, `ds-btn--{size}`. Native button attrs forwarded (`type` defaults to `button`).

### Surface

```tsx
<Surface variant="glass" padding="md">…</Surface>
```

| Prop | Values | Default |
|------|--------|---------|
| `variant` | `solid` \| `glass` \| `sunken` | `solid` |
| `padding` | `none` \| `sm` \| `md` \| `lg` | `md` |
| `as` | semantic HTML tag | `div` |

Glass respects `prefers-reduced-transparency` and missing `backdrop-filter` by falling back to elevated solid.

### Card

```tsx
<Card title="Title" description="…" footer={…} glass>
  Body
</Card>
```

| Prop | Notes |
|------|-------|
| `title` / `description` / `footer` | Optional slots |
| `glass` | boolean; uses glass material |

Classes: `ds-card`, optional `ds-card--glass`, slots `ds-card__header|title|description|body|footer`.

### Badge

```tsx
<Badge tone="success">ok</Badge>
```

| Prop | Values | Default |
|------|--------|---------|
| `tone` | `success` \| `warn` \| `danger` \| `info` \| `neutral` | `neutral` |

Classes: `ds-badge`, `ds-badge--{tone}`.

### Input

```tsx
<Input label="Name" hint="…" error="…" />
```

| Prop | Notes |
|------|-------|
| `label` | Associated via `htmlFor` / `id` |
| `hint` / `error` | Mutual description; error sets `aria-invalid` |
| `inputClassName` | Extra class on the `<input>` |

Classes: `ds-input-field`, `ds-input-label`, `ds-input`, `ds-input-hint`.

### EmptyState

```tsx
import { EmptyState, Button } from '../design-system/index.js';

<EmptyState
  tone="neutral"
  icon="◇"
  title="暂无站点"
  description="导入站点后即可管理账号与路由。"
  action={<Button size="sm" variant="primary">新建站点</Button>}
/>
```

| Prop | Values / notes | Default |
|------|----------------|---------|
| `tone` | `neutral` \| `info` \| `warn` \| `danger` | `neutral` |
| `icon` | Optional glyph / node | — |
| `title` | Required heading | — |
| `description` | Supporting copy | — |
| `action` | Single primary (or compact secondary pair) | — |

Classes: `ds-empty`, `ds-empty--{tone}`, slots `ds-empty__icon|title|description|action`.
`role="alert"` when `tone="danger"`, otherwise `role="status"`. Prefer over legacy `.empty-state` for new surfaces.

### Stack

Vertical flex layout.

| Prop | Values | Default |
|------|--------|---------|
| `gap` | `0`–`6`, `8` (space scale) | `3` |
| `align` | `start` \| `center` \| `end` \| `stretch` \| `baseline` | `stretch` |
| `justify` | `start` \| `center` \| `end` \| `between` \| `around` | `start` |

Classes: `ds-stack`, `ds-gap-*`, `ds-align-*`, `ds-justify-*`.

### Inline

Horizontal flex layout (wraps by default).

| Prop | Values | Default |
|------|--------|---------|
| `gap` | same as Stack | `2` |
| `align` | same as Stack | `center` |
| `justify` | same as Stack | `start` |
| `wrap` | boolean | `true` |

Classes: `ds-inline`, plus gap/align/justify and `ds-wrap` / `ds-nowrap`.

## Conventions

1. **Components**: import from `web/src/components/ui/**` (shadcn Base UI).
2. **Tokens**: no hard-coded brand hex in components; use OKLCH theme vars (`var(--color-*)` from `web/src/styles/theme.css`).
3. **Accessibility**: focus-visible rings, label associations, `aria-invalid` on errors, reduced-transparency fallbacks.
4. **Imports**: from `@/components/ui/*` (path alias) for new code.

## Visual acceptance

1. `cd web && bun run dev` (Rsbuild dev server) or `bun run build && bun run preview`.
2. Toggle Light / Dark; confirm primitives render correctly with the OKLCH theme tokens.

## Tests

- `web/src/**/*.test.ts(x)` — vitest unit/component tests.
- Typecheck: `cd web && bun run typecheck`
- Unit: `cd web && bun run test`
