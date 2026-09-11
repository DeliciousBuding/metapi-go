// metapi-go/context — ThemeCustomizationProvider axis contract.
//
// Every axis is the same deal: a value validated against an allowlist, read from
// a year-long cookie on first render, written back on change, mirrored onto one
// <body> attribute, and forgotten (cookie removed) when it returns to its
// default. These tests pin that contract per axis, including the two ways an
// axis legitimately differs — `font` writes the *resolved* face rather than the
// stored `default`, and `content-layout` always writes, because the stylesheet
// and the pre-paint bootstrap both address it by value.
import '@testing-library/jest-dom/vitest'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  ThemeCustomizationProvider,
  useThemeCustomization,
} from '@/context/theme-customization-provider'
import { THEME_COOKIE_KEYS } from '@/lib/theme-customization'

function clearCookies() {
  for (const key of Object.values(THEME_COOKIE_KEYS)) {
    document.cookie = `${key}=; Max-Age=0; path=/`
  }
}

function readCookie(name: string): string | undefined {
  const [, value] = `; ${document.cookie}`.split(`; ${name}=`)
  return value?.split(';')[0]
}

function renderProvider() {
  return renderHook(() => useThemeCustomization(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <ThemeCustomizationProvider>{children}</ThemeCustomizationProvider>
    ),
  })
}

beforeEach(() => {
  clearCookies()
  for (const attribute of [
    'data-theme-preset',
    'data-theme-font',
    'data-theme-radius',
    'data-theme-scale',
    'data-theme-content-layout',
  ]) {
    document.body.removeAttribute(attribute)
  }
})

afterEach(() => cleanup())

describe('ThemeCustomizationProvider axes', () => {
  it('mirrors a stored preset onto <body> and drops an unknown one', () => {
    document.cookie = `${THEME_COOKIE_KEYS.preset}=rose-garden; path=/`
    const { result, unmount } = renderProvider()

    expect(result.current.customization.preset).toBe('rose-garden')
    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')
    unmount()

    // A value outside the allowlist is not a preference, it is corrupt state:
    // fall back to the default rather than write an attribute CSS cannot match.
    document.cookie = `${THEME_COOKIE_KEYS.preset}=not-a-preset; path=/`
    const fallback = renderProvider()

    expect(fallback.result.current.customization.preset).toBe('default')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('resolves the stored font preference to the face CSS addresses', () => {
    const { result } = renderProvider()

    // `default` is a stored preference, not a CSS value: the stylesheet only
    // knows [data-theme-font='sans'] / 'serif'.
    expect(result.current.customization.font).toBe('default')
    expect(document.body).toHaveAttribute('data-theme-font', 'sans')

    act(() => result.current.setFont('serif'))
    expect(document.body).toHaveAttribute('data-theme-font', 'serif')
    expect(readCookie(THEME_COOKIE_KEYS.font)).toBe('serif')
  })

  it('persists a change and forgets the cookie when the axis returns to default', () => {
    const { result } = renderProvider()

    act(() => result.current.setRadius('lg'))
    expect(readCookie(THEME_COOKIE_KEYS.radius)).toBe('lg')
    expect(document.body).toHaveAttribute('data-theme-radius', 'lg')

    act(() => result.current.setRadius('default'))
    expect(readCookie(THEME_COOKIE_KEYS.radius)).toBeUndefined()
    expect(document.body).not.toHaveAttribute('data-theme-radius')
  })

  it('always writes content-layout, including at its default', () => {
    const { result } = renderProvider()

    expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')

    act(() => result.current.setContentLayout('centered'))
    expect(document.body).toHaveAttribute(
      'data-theme-content-layout',
      'centered'
    )
    expect(readCookie(THEME_COOKIE_KEYS.contentLayout)).toBe('centered')

    act(() => result.current.setContentLayout('full'))
    expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')
    expect(readCookie(THEME_COOKIE_KEYS.contentLayout)).toBeUndefined()
  })

  it('resets every axis and clears every cookie at once', () => {
    const { result } = renderProvider()

    act(() => {
      result.current.setPreset('ocean-breeze')
      result.current.setFont('serif')
      result.current.setRadius('xl')
      result.current.setScale('lg')
      result.current.setContentLayout('centered')
    })
    expect(result.current.customization).toEqual({
      preset: 'ocean-breeze',
      font: 'serif',
      radius: 'xl',
      scale: 'lg',
      contentLayout: 'centered',
    })

    act(() => result.current.resetCustomization())

    expect(result.current.customization).toEqual(result.current.defaults)
    for (const key of Object.values(THEME_COOKIE_KEYS)) {
      expect(readCookie(key), key).toBeUndefined()
    }
    expect(document.body).not.toHaveAttribute('data-theme-preset')
    expect(document.body).toHaveAttribute('data-theme-font', 'sans')
    expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')
  })
})
