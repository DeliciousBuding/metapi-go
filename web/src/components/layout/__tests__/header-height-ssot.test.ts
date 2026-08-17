import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

// Guards the single source of truth for the app header height.
//
// `--app-header-height` is defined once in `theme.css` and consumed everywhere
// else via `var(--app-header-height)`. These static file-content scans fail
// the moment a second, hard-coded header-height source sneaks back into the
// layout shell — whether as a Tailwind `h-14`/`h-16` utility, an inline
// `style={{ height: ... }}`, or a local `[--app-header-height:...]` arbitrary
// property that shadows the canonical token with a literal value.

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..')
const APP_HEADER_PATH = join(
  WEB_ROOT,
  'src/components/layout/components/app-header.tsx'
)
const AUTHENTICATED_LAYOUT_PATH = join(
  WEB_ROOT,
  'src/components/layout/components/authenticated-layout.tsx'
)
const LAYOUT_ERROR_BOUNDARY_PATH = join(
  WEB_ROOT,
  'src/components/layout/layout-error-boundary.tsx'
)
const THEME_CSS_PATH = join(WEB_ROOT, 'src/styles/theme.css')

// Hard-coded Tailwind height utilities that would duplicate the token. The
// `\b` boundary prevents matching `h-140`-style accidents while still
// catching the real `h-14` (3.5rem) and `h-16` (4rem) utilities.
const HARD_CODED_HEIGHT_UTILITY = /h-1[46]\b/
// A Tailwind arbitrary property that re-declares the token inline (e.g.
// `[--app-header-height:3.5rem]`), shadowing the canonical definition.
const INLINE_TOKEN_REDECLARATION = /\[--app-header-height:/
// An inline `style` attribute carrying a `height:` declaration.
const INLINE_STYLE_HEIGHT = /style=\{[^}]*height:/i

describe('header-height single source of truth', () => {
  it('app-header.tsx consumes the token instead of a hard-coded height', () => {
    const source = readFileSync(APP_HEADER_PATH, 'utf8')

    // The header root must derive its height from the token.
    expect(source).toMatch(/var\(--app-header-height\)/)

    // No second header-height source may live here.
    expect(source).not.toMatch(HARD_CODED_HEIGHT_UTILITY)
    expect(source).not.toMatch(INLINE_TOKEN_REDECLARATION)
    expect(source).not.toMatch(INLINE_STYLE_HEIGHT)
  })

  it('authenticated-layout.tsx has no hard-coded header height', () => {
    const source = readFileSync(AUTHENTICATED_LAYOUT_PATH, 'utf8')

    expect(source).not.toMatch(HARD_CODED_HEIGHT_UTILITY)
    expect(source).not.toMatch(INLINE_TOKEN_REDECLARATION)
    expect(source).not.toMatch(INLINE_STYLE_HEIGHT)
  })

  it('layout-error-boundary.tsx has no hard-coded header height', () => {
    const source = readFileSync(LAYOUT_ERROR_BOUNDARY_PATH, 'utf8')

    expect(source).not.toMatch(HARD_CODED_HEIGHT_UTILITY)
    expect(source).not.toMatch(INLINE_TOKEN_REDECLARATION)
    expect(source).not.toMatch(INLINE_STYLE_HEIGHT)
  })

  it('theme.css owns the single --app-header-height token definition', () => {
    const source = readFileSync(THEME_CSS_PATH, 'utf8')

    // The canonical source of truth must remain in theme.css so the value
    // can be swapped in one place.
    expect(source).toMatch(/--app-header-height:\s*3\.5rem/)
  })
})
