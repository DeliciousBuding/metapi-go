// metapi-go/context — direction-provider unit tests.
// Asserts DirectionProvider is the single owner of <html dir> (issue #736):
// it resolves the direction from the `dir` cookie with a localStorage
// fallback, validates the value, and applies it to document.documentElement.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'

function clearDirectionSources(): void {
  document.cookie = 'dir=; path=/; max-age=0'
  localStorage.removeItem('dir')
  document.documentElement.removeAttribute('dir')
}

beforeEach(() => {
  clearDirectionSources()
})

afterEach(() => {
  cleanup()
  clearDirectionSources()
})

function renderProvider() {
  return render(
    <DirectionProvider>
      <div>child</div>
    </DirectionProvider>
  )
}

describe('DirectionProvider dir ownership', () => {
  it('defaults to ltr when no direction is stored', () => {
    renderProvider()
    expect(document.documentElement).toHaveAttribute('dir', 'ltr')
  })

  it('resolves dir from the cookie', () => {
    document.cookie = 'dir=rtl; path=/'
    renderProvider()
    expect(document.documentElement).toHaveAttribute('dir', 'rtl')
  })

  it('falls back to localStorage when no cookie is set', () => {
    localStorage.setItem('dir', 'rtl')
    renderProvider()
    expect(document.documentElement).toHaveAttribute('dir', 'rtl')
  })

  it('prefers the cookie over localStorage', () => {
    document.cookie = 'dir=ltr; path=/'
    localStorage.setItem('dir', 'rtl')
    renderProvider()
    expect(document.documentElement).toHaveAttribute('dir', 'ltr')
  })

  it('ignores invalid stored values and falls back to ltr', () => {
    document.cookie = 'dir=sideways; path=/'
    renderProvider()
    expect(document.documentElement).toHaveAttribute('dir', 'ltr')
  })
})
