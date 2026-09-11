// metapi-go/context — ThemeCustomizationProvider: the appearance axes that sit
// on top of the colour scheme (preset / font / radius / scale) plus the content
// layout, mirrored onto <body> as data attributes so `styles/theme-presets.css`
// can override the design tokens at the right point in the cascade.
//
// Every axis is the same four things: a value validated against an allowlist,
// read from a year-long cookie on first render, written back on change, and
// mirrored onto one <body> attribute. `usePersistedAxis` owns that once, so the
// provider reads as five declarations plus the one axis that needs something
// extra — a preset changes `--background`, so `<meta name="theme-color">` has to
// follow it.
//
// Ordering note: `public/theme-init.js` writes the same attributes before React
// mounts (that is the whole point of cookies over localStorage here), so this
// provider is *confirming* the pre-paint state, not establishing it. A value the
// bootstrap does not know is dropped back to the default by the allowlist.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'

import { getCookie, removeCookie, setCookie } from '@/lib/cookies'
import { syncThemeColorMeta } from '@/lib/theme-color-meta'
import {
  CONTENT_LAYOUT_VALUES,
  type ContentLayout,
  DEFAULT_THEME_CUSTOMIZATION,
  resolveThemeFont,
  THEME_COOKIE_KEYS,
  THEME_FONT_VALUES,
  THEME_PRESET_VALUES,
  THEME_RADIUS_VALUES,
  THEME_SCALE_VALUES,
  type ThemeCustomization,
  type ThemeFont,
  type ThemePreset,
  type ThemeRadius,
  type ThemeScale,
} from '@/lib/theme-customization'

const COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year

type Axis<T extends string> = {
  /** The <body> attribute this axis mirrors onto. */
  attribute: string
  /** The cookie the choice persists to. */
  cookieKey: string
  default: T
  values: ReadonlySet<T>
  /**
   * Stored preference → what CSS addresses; identity when omitted. Returning
   * `null` removes the attribute, which is how an axis hands control back to
   * `theme.css` instead of restating its value from a preset.
   */
  toAttribute?: (value: T) => string | null
}

/**
 * The three axes `theme-presets.css` overrides: at `default` they write nothing
 * at all, so the tokens in `theme.css` win rather than a preset restating them.
 */
function omitAtDefault(value: string): string | null {
  return value === 'default' ? null : value
}

const PRESET_AXIS: Axis<ThemePreset> = {
  attribute: 'data-theme-preset',
  cookieKey: THEME_COOKIE_KEYS.preset,
  default: DEFAULT_THEME_CUSTOMIZATION.preset,
  values: THEME_PRESET_VALUES,
  toAttribute: omitAtDefault,
}

// The font axis has no `default` in CSS: the stylesheet addresses the concrete
// face, so the preference is resolved on the way out.
const FONT_AXIS: Axis<ThemeFont> = {
  attribute: 'data-theme-font',
  cookieKey: THEME_COOKIE_KEYS.font,
  default: DEFAULT_THEME_CUSTOMIZATION.font,
  values: THEME_FONT_VALUES,
  toAttribute: resolveThemeFont,
}

const RADIUS_AXIS: Axis<ThemeRadius> = {
  attribute: 'data-theme-radius',
  cookieKey: THEME_COOKIE_KEYS.radius,
  default: DEFAULT_THEME_CUSTOMIZATION.radius,
  values: THEME_RADIUS_VALUES,
  toAttribute: omitAtDefault,
}

const SCALE_AXIS: Axis<ThemeScale> = {
  attribute: 'data-theme-scale',
  cookieKey: THEME_COOKIE_KEYS.scale,
  default: DEFAULT_THEME_CUSTOMIZATION.scale,
  values: THEME_SCALE_VALUES,
  toAttribute: omitAtDefault,
}

// Always written, including `full`: the layout stylesheet and the pre-paint
// bootstrap both address this attribute by value.
const CONTENT_LAYOUT_AXIS: Axis<ContentLayout> = {
  attribute: 'data-theme-content-layout',
  cookieKey: THEME_COOKIE_KEYS.contentLayout,
  default: DEFAULT_THEME_CUSTOMIZATION.contentLayout,
  values: CONTENT_LAYOUT_VALUES,
}

/** The allowlisted value in `cookieKey`, or `fallback` when absent/unknown. */
function readAxis<T extends string>(
  cookieKey: string,
  values: ReadonlySet<T>,
  fallback: T
): T {
  const stored = getCookie(cookieKey)
  return stored && values.has(stored as T) ? (stored as T) : fallback
}

/**
 * One persisted appearance axis: cookie-backed state that mirrors itself onto a
 * <body> attribute, and forgets the cookie when it returns to the default.
 */
function usePersistedAxis<T extends string>(
  axis: Axis<T>
): [T, (value: T) => void] {
  const [value, setValue] = useState<T>(() =>
    readAxis(axis.cookieKey, axis.values, axis.default)
  )

  useEffect(() => {
    const written = axis.toAttribute ? axis.toAttribute(value) : value

    if (written === null) document.body.removeAttribute(axis.attribute)
    else document.body.setAttribute(axis.attribute, written)
  }, [axis, value])

  const persist = useCallback(
    (next: T) => {
      setValue(next)
      if (next === axis.default) removeCookie(axis.cookieKey)
      else setCookie(axis.cookieKey, next, COOKIE_MAX_AGE)
    },
    [axis]
  )

  return [value, persist]
}

type ThemeCustomizationContextValue = {
  defaults: ThemeCustomization
  customization: ThemeCustomization
  setPreset: (preset: ThemePreset) => void
  setFont: (font: ThemeFont) => void
  setRadius: (radius: ThemeRadius) => void
  setScale: (scale: ThemeScale) => void
  setContentLayout: (contentLayout: ContentLayout) => void
  resetCustomization: () => void
}

/**
 * Seen by a consumer rendered outside a provider (an error boundary mounted
 * before the provider stack is ready): every axis reads as its default and
 * writes are ignored, so a stray `useThemeCustomization()` degrades rather than
 * crashing.
 */
const UNMOUNTED: ThemeCustomizationContextValue = {
  defaults: DEFAULT_THEME_CUSTOMIZATION,
  customization: DEFAULT_THEME_CUSTOMIZATION,
  setPreset: () => {},
  setFont: () => {},
  setRadius: () => {},
  setScale: () => {},
  setContentLayout: () => {},
  resetCustomization: () => {},
}

const ThemeCustomizationContext =
  createContext<ThemeCustomizationContextValue>(UNMOUNTED)

export function ThemeCustomizationProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [preset, setPreset] = usePersistedAxis(PRESET_AXIS)
  const [font, setFont] = usePersistedAxis(FONT_AXIS)
  const [radius, setRadius] = usePersistedAxis(RADIUS_AXIS)
  const [scale, setScale] = usePersistedAxis(SCALE_AXIS)
  const [contentLayout, setContentLayout] =
    usePersistedAxis(CONTENT_LAYOUT_AXIS)

  // A preset redefines --background, so the browser chrome colour has to follow.
  useEffect(() => {
    syncThemeColorMeta()
  }, [preset])

  const resetCustomization = useCallback(() => {
    setPreset(DEFAULT_THEME_CUSTOMIZATION.preset)
    setFont(DEFAULT_THEME_CUSTOMIZATION.font)
    setRadius(DEFAULT_THEME_CUSTOMIZATION.radius)
    setScale(DEFAULT_THEME_CUSTOMIZATION.scale)
    setContentLayout(DEFAULT_THEME_CUSTOMIZATION.contentLayout)
  }, [setPreset, setFont, setRadius, setScale, setContentLayout])

  const value = useMemo<ThemeCustomizationContextValue>(
    () => ({
      defaults: DEFAULT_THEME_CUSTOMIZATION,
      customization: { preset, font, radius, scale, contentLayout },
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetCustomization,
    }),
    [
      preset,
      font,
      radius,
      scale,
      contentLayout,
      setPreset,
      setFont,
      setRadius,
      setScale,
      setContentLayout,
      resetCustomization,
    ]
  )

  return (
    <ThemeCustomizationContext value={value}>
      {children}
    </ThemeCustomizationContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useThemeCustomization(): ThemeCustomizationContextValue {
  return useContext(ThemeCustomizationContext)
}
