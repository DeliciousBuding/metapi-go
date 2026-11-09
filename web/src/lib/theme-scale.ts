// The display-size cookie and values also belong to public/theme-init.js,
// which applies the same preference before the application mounts.
export type ThemeScale = 'default' | 'sm' | 'lg' | 'xl'

export const THEME_SCALE_COOKIE = 'theme_scale'
export const THEME_SCALE_VALUES: ReadonlySet<ThemeScale> = new Set([
  'default',
  'sm',
  'lg',
  'xl',
])
