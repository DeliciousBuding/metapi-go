// metapi-go/context — ThemeProvider: the light / dark / system colour scheme.
//
// Three jobs, in the order they matter:
//   1. resolve `system` against `prefers-color-scheme` and keep resolving it —
//      the media-query listener is what makes "follow system" actually follow;
//   2. publish the resolved scheme everywhere something reads it: the
//      `.light` / `.dark` class (Tailwind 4 dark mode is class-based),
//      `data-theme`, `color-scheme` (native form controls and scrollbars), and
//      `<meta name="theme-color">` (browser chrome);
//   3. persist the choice in a year-long cookie, which is what lets
//      `public/bootstrap.js` paint the right background before the bundle loads.
//
// On mount it also strips the bootstrap's inline background from `<html>`: that
// inline style exists only to win the first paint, and leaving it in place makes
// overscroll reveal the pre-mount colour instead of the themed body background
// (which propagates to the canvas).

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

export type Theme = 'dark' | 'light' | 'system'
export type ResolvedTheme = Exclude<Theme, 'system'>

const DEFAULT_THEME: Theme = 'system'
const THEME_COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year
const THEMES = new Set<Theme>(['dark', 'light', 'system'])
const PREFERS_DARK = '(prefers-color-scheme: dark)'
// Legacy cookie name, kept deliberately: it is a year of client state, and it is
// what `public/bootstrap.js` reads before this module exists.
const THEME_COOKIE_NAME = 'vite-ui-theme'

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeContextValue = {
  defaultTheme: Theme
  resolvedTheme: ResolvedTheme
  theme: Theme
  setTheme: (theme: Theme) => void
  resetTheme: () => void
}

/**
 * Value seen by a consumer rendered outside a provider: reads as the light
 * default and ignores writes. Not hypothetical — `ui/__tests__/axe-toast`
 * renders `<Toaster>` standalone, and an error boundary can mount before the
 * provider stack is ready. A stray `useTheme()` should degrade, not crash.
 */
const UNMOUNTED: ThemeContextValue = {
  defaultTheme: DEFAULT_THEME,
  resolvedTheme: 'light',
  theme: DEFAULT_THEME,
  setTheme: () => {},
  resetTheme: () => {},
}

const ThemeContext = createContext<ThemeContextValue>(UNMOUNTED)

function systemTheme(): ResolvedTheme {
  return window.matchMedia(PREFERS_DARK).matches ? 'dark' : 'light'
}

function resolve(theme: Theme): ResolvedTheme {
  return theme === 'system' ? systemTheme() : theme
}

/** The persisted choice, or `fallback` when it is absent or not a known theme. */
function readStoredTheme(storageKey: string, fallback: Theme): Theme {
  const stored = getCookie(storageKey) as Theme | undefined
  return stored && THEMES.has(stored) ? stored : fallback
}

export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  storageKey = THEME_COOKIE_NAME,
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(() =>
    readStoredTheme(storageKey, defaultTheme)
  )
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() =>
    resolve(readStoredTheme(storageKey, defaultTheme))
  )

  useEffect(() => {
    const root = document.documentElement
    const prefersDark = window.matchMedia(PREFERS_DARK)

    const apply = () => {
      const next = resolve(theme)

      root.classList.remove('light', 'dark')
      root.classList.add(next)
      root.setAttribute('data-theme', next)
      root.style.colorScheme = next
      setResolvedTheme(next)
      syncThemeColorMeta()
    }

    apply()

    root.style.removeProperty('background-color')
    root.style.removeProperty('--bootstrap-background')

    prefersDark.addEventListener('change', apply)
    return () => prefersDark.removeEventListener('change', apply)
  }, [theme])

  const setTheme = useCallback(
    (next: Theme) => {
      setCookie(storageKey, next, THEME_COOKIE_MAX_AGE)
      setThemeState(next)
    },
    [storageKey]
  )

  /** Back to following the system, and forget the stored override. */
  const resetTheme = useCallback(() => {
    removeCookie(storageKey)
    setThemeState(defaultTheme)
  }, [defaultTheme, storageKey])

  const value = useMemo<ThemeContextValue>(
    () => ({ defaultTheme, resolvedTheme, resetTheme, setTheme, theme }),
    [defaultTheme, resolvedTheme, resetTheme, setTheme, theme]
  )

  return <ThemeContext value={value}>{children}</ThemeContext>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme(): ThemeContextValue {
  return useContext(ThemeContext)
}
