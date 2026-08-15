// metapi-go FOUC bootstrap alignment gate.
// index.html ships inline bootstrap scripts that set `data-theme-*` attributes
// before first paint. The hardcoded allowlists in those scripts must stay in
// sync with the theme constants (`THEME_PRESETS` / radius / scale) — a new
// value added to the constants but forgotten here would flash the default
// theme for one frame before React mounts.

import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import {
  THEME_PRESETS,
  THEME_RADIUS_VALUES,
  THEME_SCALE_VALUES,
} from '../../lib/theme-customization'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const indexHtml = readFileSync(join(WEB_ROOT, 'index.html'), 'utf8')

/** Extract the string literals of the first capture group of `regex`. */
function extractStringList(regex: RegExp): string[] {
  const match = indexHtml.match(regex)
  expect(match).toBeTruthy()
  const body = match?.[1] ?? ''
  return [...body.matchAll(/'([^']+)'/g)].map((m) => m[1])
}

const isNotDefault = (value: string) => value !== 'default'

describe('FOUC bootstrap ↔ theme constants', () => {
  it('preset allowlist matches THEME_PRESETS minus the default preset', () => {
    const bootstrapPresets = extractStringList(/var presets = \[([\s\S]*?)\]/)
    const expected = THEME_PRESETS.map((p) => p.value).filter(isNotDefault)
    expect(bootstrapPresets).toEqual(expected)
  })

  it('radius allowlist matches THEME_RADIUS_VALUES minus default', () => {
    const bootstrapRadii = extractStringList(
      /\[('none'[^\]]*)\]\.indexOf\(radius\)/
    )
    const expected = [...THEME_RADIUS_VALUES].filter(isNotDefault)
    expect(bootstrapRadii).toEqual(expected)
  })

  it('scale allowlist matches THEME_SCALE_VALUES minus default', () => {
    const bootstrapScales = extractStringList(
      /\[('sm'[^\]]*)\]\.indexOf\(scale\)/
    )
    const expected = [...THEME_SCALE_VALUES].filter(isNotDefault)
    expect(bootstrapScales).toEqual(expected)
  })
})
