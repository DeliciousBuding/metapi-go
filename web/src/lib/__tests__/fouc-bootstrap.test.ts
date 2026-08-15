// metapi-go — FOUC bootstrap lockstep gate.
// index.html resolves the theme preset and font before React mounts via a
// hand-written script. `THEME_PRESETS` / `resolveThemeFont` (this module's
// neighbours) are the source of truth; these tests read index.html from disk
// and fail when the bootstrap drifts from the constants — e.g. a preset added
// to THEME_PRESETS but forgotten in the bootstrap list.

import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { afterEach, describe, expect, it } from 'vitest'

import {
  resolveThemeFont,
  THEME_PRESETS,
  type ThemeFont,
} from '../theme-customization'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const INDEX_HTML = readFileSync(join(WEB_ROOT, 'index.html'), 'utf8')

const PRESET_IDS = THEME_PRESETS.map((preset) => preset.value)
const FONT_VALUES: ThemeFont[] = ['default', 'sans', 'serif']

// Every <body> attribute the bootstrap scripts may touch — restored after
// each test so scenarios cannot leak into one another.
const BODY_THEME_ATTRIBUTES = [
  'data-theme-preset',
  'data-theme-font',
  'data-theme-radius',
  'data-theme-scale',
  'data-theme-content-layout',
]

const BOOTSTRAP_COOKIE_NAMES = ['theme_preset', 'theme_font']

/** Pulls the string literals out of the bootstrap `var presets = [...]` list. */
function extractBootstrapPresets(): string[] {
  const match = INDEX_HTML.match(/var\s+presets\s*=\s*\[([\s\S]*?)\]/)
  if (!match) {
    throw new Error('index.html bootstrap is missing the `presets` list')
  }
  const literals = [...match[1].matchAll(/'([^']+)'/g)]
  if (literals.length === 0) {
    throw new Error('index.html bootstrap preset list has no entries')
  }
  return literals.map((literal) => literal[1])
}

/** Collects every inline `<script>` body from index.html. */
function extractInlineScripts(): string[] {
  const scripts = [...INDEX_HTML.matchAll(/<script>([\s\S]*?)<\/script>/g)]
  if (scripts.length === 0) {
    throw new Error('index.html contains no inline bootstrap scripts')
  }
  return scripts.map((script) => script[1].trim())
}

function setBootstrapCookies(cookies: Record<string, string>) {
  for (const name of BOOTSTRAP_COOKIE_NAMES) {
    if (cookies[name] !== undefined) {
      document.cookie = `${name}=${cookies[name]}`
    } else {
      document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT`
    }
  }
}

/**
 * Runs the real index.html bootstrap scripts against the vitest jsdom
 * document. The scripts are IIFEs that read `document` / `window` as free
 * variables — exactly what the browser provides as globals — so they are
 * executed verbatim with those two bindings injected.
 */
function runBootstrap(cookies: Record<string, string>): Document {
  setBootstrapCookies(cookies)
  for (const script of extractInlineScripts()) {
    const execute = new Function('document', 'window', script) as (
      documentRef: Document,
      windowRef: Window
    ) => void
    execute(document, window)
  }
  return document
}

afterEach(() => {
  for (const name of BODY_THEME_ATTRIBUTES) {
    document.body.removeAttribute(name)
  }
  const html = document.documentElement
  html.classList.remove('light', 'dark', 'null')
  html.style.removeProperty('color-scheme')
  html.style.removeProperty('--bootstrap-background')
  html.style.removeProperty('background-color')
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', '#ffffff')
  setBootstrapCookies({})
})

describe('FOUC bootstrap lockstep', () => {
  it('lists every THEME_PRESETS id in the bootstrap, in the same order', () => {
    expect(extractBootstrapPresets()).toEqual(PRESET_IDS)
  })

  it('applies the preset attribute for every valid preset cookie', () => {
    for (const presetId of PRESET_IDS) {
      const bootstrapDocument = runBootstrap({ theme_preset: presetId })
      expect(bootstrapDocument.body.getAttribute('data-theme-preset')).toBe(
        presetId
      )
    }
  })

  it('resolves the font cookie exactly like resolveThemeFont', () => {
    for (const presetId of PRESET_IDS) {
      for (const font of FONT_VALUES) {
        const bootstrapDocument = runBootstrap({
          theme_preset: presetId,
          theme_font: font,
        })
        expect(
          bootstrapDocument.body.getAttribute('data-theme-font'),
          `preset=${presetId} font=${font}`
        ).toBe(resolveThemeFont(font, presetId))
      }
    }
  })

  it('falls back to sans with no preset/font cookies stored', () => {
    const bootstrapDocument = runBootstrap({})
    expect(bootstrapDocument.body.getAttribute('data-theme-font')).toBe('sans')
    expect(bootstrapDocument.body.getAttribute('data-theme-preset')).toBeNull()
  })
})
