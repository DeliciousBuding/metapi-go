// metapi-go/lib — index.html theme bootstrap parity guard.
//
// The FOUC bootstrap script in index.html carries its own hardcoded copy of
// the preset whitelist (it must run before any bundled module loads, so it
// cannot import the TS constants). This test fails when the two lists drift:
// every non-default preset in lib/theme-customization.ts must appear in the
// bootstrap list and vice versa.

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { THEME_PRESETS, THEME_PRESET_VALUES } from '../theme-customization'

// Tests always run from the web/ project root (`bun run test`).
const INDEX_HTML_PATH = resolve(process.cwd(), 'index.html')

function extractBootstrapPresetList(html: string): string[] {
  const listMatch = html.match(/var presets = \[([\s\S]*?)\]/)
  if (!listMatch) {
    throw new Error('preset whitelist not found in index.html bootstrap')
  }
  return [...listMatch[1].matchAll(/'([^']+)'/g)].map((match) => match[1])
}

describe('index.html theme bootstrap preset whitelist', () => {
  const html = readFileSync(INDEX_HTML_PATH, 'utf-8')
  const bootstrapPresets = extractBootstrapPresetList(html)

  it('lists each preset exactly once', () => {
    expect(bootstrapPresets.length).toBeGreaterThan(0)
    expect(new Set(bootstrapPresets).size).toBe(bootstrapPresets.length)
  })

  it('only contains presets known to the TS constants', () => {
    const unknownPresets = bootstrapPresets.filter(
      (preset) => !THEME_PRESET_VALUES.has(preset as never)
    )
    expect(unknownPresets).toEqual([])
  })

  it('covers every non-default preset from the TS constants', () => {
    // `default` is intentionally absent: the bootstrap only sets the body
    // attribute for customized presets, and the provider removes it for the
    // default preset.
    const missingPresets = THEME_PRESETS.map((preset) => preset.value)
      .filter((value) => value !== 'default')
      .filter((value) => !bootstrapPresets.includes(value))
    expect(missingPresets).toEqual([])
  })
})
