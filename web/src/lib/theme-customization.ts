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
    swatches: ['oklch(0.72 0.18 250)', 'oklch(0.7 0.12 280)'],
  },
  {
    // Warm cream canvas with clay/coral as the single accent.
    value: 'anthropic',
    name: 'Anthropic',
    swatches: ['oklch(0.984 0.005 95)', 'oklch(0.57 0.15 38)'],
  },
  {
    value: 'simple-large',
    name: 'Simple',
    swatches: ['oklch(0.15 0 0)', 'oklch(0.99 0 0)'],
  },
  {
    value: 'underground',
    name: 'Underground',
    swatches: ['oklch(0.5315 0.0694 156.19)', 'oklch(0.5748 0.0862 336.52)'],
  },
  {
    value: 'rose-garden',
    name: 'Rose Garden',
    swatches: ['oklch(0.5827 0.2418 12.23)', 'oklch(0.8131 0.1129 5.67)'],
  },
  {
    value: 'lake-view',
    name: 'Lake View',
    swatches: ['oklch(0.765 0.177 163.22)', 'oklch(0.551 0.0899 200.52)'],
  },
  {
    value: 'sunset-glow',
    name: 'Sunset Glow',
    swatches: ['oklch(0.5591 0.1882 25.33)', 'oklch(0.7938 0.1248 42.42)'],
  },
  {
    value: 'forest-whisper',
    name: 'Forest Whisper',
    swatches: ['oklch(0.5276 0.1072 182.22)', 'oklch(0.5236 0.0505 250.18)'],
  },
  {
    value: 'ocean-breeze',
    name: 'Ocean Breeze',
    swatches: ['oklch(0.5461 0.2152 262.88)', 'oklch(0.5854 0.2041 277.12)'],
  },
  {
    value: 'lavender-dream',
    name: 'Lavender Dream',
    swatches: ['oklch(0.5709 0.1808 306.89)', 'oklch(0.811 0.0589 201.14)'],
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
