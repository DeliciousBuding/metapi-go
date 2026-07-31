/**
 * Theme FOUC bootstrap helpers (#535).
 * Pure functions shared by unit tests; index.html inlines the same resolution rules.
 */

export const THEME_MODE_KEY = 'theme_mode';
export const LEGACY_THEME_KEY = 'theme';
export const THEME_ACCENT_KEY = 'theme_accent';

export type ThemeMode = 'system' | 'light' | 'dark';
export type DataTheme = 'light' | 'dark';

/** VIS-1: selectable brand-primary families (see tokens.css data-accent). */
export type AccentPreset = 'blue' | 'indigo' | 'teal';
export const ACCENT_PRESETS: AccentPreset[] = ['blue', 'indigo', 'teal'];

export type ThemeStorageGetItem = (key: string) => string | null;

/** cloud-ops canvas (tokendance-design/styles/cloud-ops) — keep FOUC in sync with tokens.css */
const CANVAS_BG_LIGHT = '#f8f9fa';
const CANVAS_BG_DARK = '#202124';

function isDataTheme(value: string | null | undefined): value is DataTheme {
  return value === 'light' || value === 'dark';
}

function isAccentPreset(value: string | null | undefined): value is AccentPreset {
  return value === 'blue' || value === 'indigo' || value === 'teal';
}

/**
 * Resolve the initial `data-accent` value before React hydrates.
 * Unknown values fall back to the default 'blue'.
 */
export function resolveInitialAccent(getItem: ThemeStorageGetItem): AccentPreset {
  const value = getItem(THEME_ACCENT_KEY);
  return isAccentPreset(value) ? value : 'blue';
}

/**
 * Resolve the initial `data-theme` value before React hydrates.
 * Priority: theme_mode (light|dark) → theme_mode=system + prefersDark → legacy theme → prefersDark.
 */
export function resolveInitialDataTheme(
  getItem: ThemeStorageGetItem,
  prefersDark: boolean,
): DataTheme {
  const mode = getItem(THEME_MODE_KEY);
  if (isDataTheme(mode)) return mode;
  if (mode === 'system') return prefersDark ? 'dark' : 'light';

  const legacy = getItem(LEGACY_THEME_KEY);
  if (isDataTheme(legacy)) return legacy;

  return prefersDark ? 'dark' : 'light';
}

/** Solid canvas color applied before CSS tokens load (prevents white flash). */
export function canvasBackgroundForTheme(theme: DataTheme): string {
  return theme === 'dark' ? CANVAS_BG_DARK : CANVAS_BG_LIGHT;
}
