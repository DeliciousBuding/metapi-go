// metapi-go/lib — the theme customization vocabulary: the axes a user can
// change, their allowed values, and where each choice is persisted.
//
// Lives in `lib/` rather than beside the provider so both the provider and the
// customizer panel can import it without crossing the React Fast Refresh
// boundary (`only-export-components`).
//
// Two hard constraints on this module:
//
//   1. `public/theme-init.js` applies these choices before first paint, so it
//      carries its own hardcoded copy of the allowlists (it runs before any
//      bundled module exists and cannot import from here). Two gates fail if the
//      copies drift — `styles/__tests__/fouc-bootstrap` and
//      `lib/__tests__/theme-bootstrap-parity` — and both compare **in order**,
//      so declaration order below is part of the contract, not a style choice.
//   2. The cookie keys and every `value` string are persisted client state
//      (one-year cookies). Renaming one silently resets that user's appearance.
//
// The preset *palette* — the `swatches` below and the oklch values they preview
// in `styles/theme-presets.css` — is design data rather than logic, and is the
// product's visual identity; changing it is a design decision, not a refactor.

/**
 * The colour presets, in customizer display order. Each entry carries the two
 * swatches its card previews (the preset's `--primary` and `--secondary` from
 * `theme-presets.css`).
 */
export const THEME_PRESETS = [
  {
    value: 'default',
    name: 'Default',
    swatches: ['oklch(0.565 0.19 262)', 'oklch(0.47 0.16 200)'],
  },
  {
    // Achromatic high-contrast preset; pairs with the density scale axis for
    // the accessibility-oriented reading configuration.
    value: 'graphite',
    name: 'Graphite',
    swatches: ['oklch(0.24 0 0)', 'oklch(0.925 0 0)'],
  },
  {
    value: 'cobalt',
    name: 'Cobalt',
    swatches: ['oklch(0.53 0.17 230)', 'oklch(0.57 0.11 20)'],
  },
  {
    value: 'lagoon',
    name: 'Lagoon',
    swatches: ['oklch(0.51 0.17 196)', 'oklch(0.57 0.11 346)'],
  },
  {
    value: 'kelp',
    name: 'Kelp',
    swatches: ['oklch(0.525 0.17 156)', 'oklch(0.565 0.11 306)'],
  },
  {
    value: 'moss',
    name: 'Moss',
    swatches: ['oklch(0.545 0.17 124)', 'oklch(0.56 0.11 274)'],
  },
  {
    value: 'ochre',
    name: 'Ochre',
    swatches: ['oklch(0.555 0.17 85)', 'oklch(0.55 0.11 235)'],
  },
  {
    value: 'ember',
    name: 'Ember',
    swatches: ['oklch(0.575 0.17 40)', 'oklch(0.535 0.11 190)'],
  },
  {
    value: 'berry',
    name: 'Berry',
    swatches: ['oklch(0.58 0.17 350)', 'oklch(0.545 0.11 140)'],
  },
  {
    value: 'plum',
    name: 'Plum',
    swatches: ['oklch(0.575 0.17 314)', 'oklch(0.555 0.11 104)'],
  },
] as const

export type ThemePreset = (typeof THEME_PRESETS)[number]['value']
export type ThemeRadius = 'default' | 'none' | 'sm' | 'md' | 'lg' | 'xl'
export type ThemeScale = 'default' | 'sm' | 'lg' | 'xl'
export type ContentLayout = 'full' | 'centered'

/**
 * The body-font axis.
 *
 * `default` is what ships and what a reset returns to; it resolves to `sans`
 * (the humanist Public Sans voice) for every preset, so serif only ever appears
 * as an explicit user choice. `resolveThemeFont` is the single place that
 * resolution happens — CSS addresses the concrete face
 * (`[data-theme-font='sans']` / `='serif'`), never `default`.
 */
export type ThemeFont = 'default' | 'sans' | 'serif'

export type ResolvedThemeFont = Exclude<ThemeFont, 'default'>

export type ThemeCustomization = {
  preset: ThemePreset
  font: ThemeFont
  radius: ThemeRadius
  scale: ThemeScale
  contentLayout: ContentLayout
}

export const DEFAULT_THEME_CUSTOMIZATION: ThemeCustomization = {
  preset: 'default',
  font: 'default',
  radius: 'default',
  scale: 'default',
  contentLayout: 'full',
}

export const THEME_PRESET_VALUES = new Set(
  THEME_PRESETS.map((preset) => preset.value)
) as ReadonlySet<ThemePreset>

export const THEME_FONT_VALUES: ReadonlySet<ThemeFont> = new Set([
  'default',
  'sans',
  'serif',
])

export const THEME_RADIUS_VALUES: ReadonlySet<ThemeRadius> = new Set([
  'default',
  'none',
  'sm',
  'md',
  'lg',
  'xl',
])

export const THEME_SCALE_VALUES: ReadonlySet<ThemeScale> = new Set([
  'default',
  'sm',
  'lg',
  'xl',
])

export const CONTENT_LAYOUT_VALUES: ReadonlySet<ContentLayout> = new Set([
  'full',
  'centered',
])

/**
 * Cookie each axis persists to. Read by `public/theme-init.js` before the
 * bundle loads — the strings are a contract with that script, not free names.
 */
export const THEME_COOKIE_KEYS = {
  preset: 'theme_preset',
  font: 'theme_font',
  radius: 'theme_radius',
  scale: 'theme_scale',
  contentLayout: 'theme_content_layout',
} as const

/** The stored font preference as the concrete face CSS addresses. */
export function resolveThemeFont(font: ThemeFont): ResolvedThemeFont {
  return font === 'default' ? 'sans' : font
}
