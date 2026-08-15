// metapi-go/context — direction-provider ported from newapi. AGPL header stripped.
// RTL provider: sets <html dir> attribute + wraps Base UI DirectionProvider.
// This is the single owner of `document.documentElement.dir` — the direction
// is resolved from the `dir` cookie with a localStorage fallback, and nothing
// else (e.g. i18n language sync) may write the attribute.

import { DirectionProvider as BaseDirectionProvider } from '@base-ui/react/direction-provider'
import { createContext, useEffect, useState } from 'react'

import { getCookie, removeCookie, setCookie } from '@/lib/cookies'

export type Direction = 'ltr' | 'rtl'

const DEFAULT_DIRECTION = 'ltr'
const DIRECTION_COOKIE_NAME = 'dir'
const DIRECTION_STORAGE_KEY = 'dir'
const DIRECTION_COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year

type DirectionContextType = {
  defaultDir: Direction
  dir: Direction
  setDir: (dir: Direction) => void
  resetDir: () => void
}

const DirectionContext = createContext<DirectionContextType | null>(null)

function isDirection(value: string | null | undefined): value is Direction {
  return value === 'ltr' || value === 'rtl'
}

/** Cookie wins; localStorage is the fallback for clients without cookies. */
function readStoredDirection(): Direction {
  const cookieDirection = getCookie(DIRECTION_COOKIE_NAME)
  if (isDirection(cookieDirection)) return cookieDirection

  try {
    const storageDirection = window.localStorage.getItem(DIRECTION_STORAGE_KEY)
    if (isDirection(storageDirection)) return storageDirection
  } catch {
    // Storage unavailable (private mode etc.) — fall through to the default.
  }

  return DEFAULT_DIRECTION
}

function persistDirection(dir: Direction): void {
  setCookie(DIRECTION_COOKIE_NAME, dir, DIRECTION_COOKIE_MAX_AGE)
  try {
    window.localStorage.setItem(DIRECTION_STORAGE_KEY, dir)
  } catch {
    // Non-fatal: the cookie already persists the choice.
  }
}

function clearStoredDirection(): void {
  removeCookie(DIRECTION_COOKIE_NAME)
  try {
    window.localStorage.removeItem(DIRECTION_STORAGE_KEY)
  } catch {
    // Non-fatal: the cookie is the primary source.
  }
}

export function DirectionProvider({ children }: { children: React.ReactNode }) {
  const [dir, _setDir] = useState<Direction>(readStoredDirection)

  useEffect(() => {
    const htmlElement = document.documentElement
    htmlElement.setAttribute('dir', dir)
  }, [dir])

  const setDir = (dir: Direction) => {
    _setDir(dir)
    persistDirection(dir)
  }

  const resetDir = () => {
    _setDir(DEFAULT_DIRECTION)
    clearStoredDirection()
  }

  return (
    <DirectionContext
      value={{
        defaultDir: DEFAULT_DIRECTION,
        dir,
        setDir,
        resetDir,
      }}
    >
      <BaseDirectionProvider direction={dir}>{children}</BaseDirectionProvider>
    </DirectionContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
