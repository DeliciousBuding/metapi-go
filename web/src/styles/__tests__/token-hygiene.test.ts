// metapi-go design-token hygiene gate.
// Locks in the token-first contract: status colors must resolve through
// semantic tokens (--success/--warning/--destructive/--info), fallback
// avatars must reference tokens that actually exist, and theme.css must
// define the overlay/shadow tokens consumed by primitives.

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const SRC = join(WEB_ROOT, 'src')

function walk(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (entry === 'node_modules' || entry === '__tests__') continue
      out.push(...walk(full))
    } else if (/\.(ts|tsx)$/.test(entry) && !/\.test\./.test(entry)) {
      out.push(full)
    }
  }
  return out
}

const allSource = walk(SRC).map((file) => ({
  file,
  text: readFileSync(file, 'utf8'),
}))

const read = (path: string) => readFileSync(join(WEB_ROOT, path), 'utf8')

describe('design-token hygiene', () => {
  it('status colors resolve through semantic tokens, not Tailwind palette', () => {
    const offenders = allSource
      .filter(({ text }) =>
        /bg-emerald-|text-emerald-|bg-red-500|bg-amber-500|bg-blue-500|text-red-500|text-amber-500/.test(
          text
        )
      )
      .map(({ file }) => file.slice(SRC.length + 1))
    expect(offenders).toEqual([])
  })

  it('fallback/avatar colors reference tokens that exist', () => {
    const text = allSource.map(({ text }) => text).join('\n')
    expect(text).not.toMatch(/var\(--color-on-primary\)/)
    expect(text).not.toMatch(/--color-chart-[67]/)
    expect(text).not.toMatch(/--color-stat-(orange|cyan)-ink/)
  })

  it('theme.css defines the overlay and card-hover tokens consumed by primitives', () => {
    const theme = read('src/styles/theme.css')
    expect(theme).toContain('--color-overlay: var(--overlay)')
    expect(theme).toContain('--overlay:')
    expect(theme).toContain('--shadow-card-hover:')
    // no dead overview-accent tokens
    expect(theme).not.toContain('overview-accent')
  })

  it('theme presets stop aliasing status colors to chart series', () => {
    const presets = read('src/styles/theme-presets.css')
    expect(presets).not.toMatch(/--success: var\(--chart-2\)/)
    expect(presets).not.toMatch(/--warning: var\(--chart-4\)/)
    expect(presets).not.toMatch(/--info: var\(--chart-1\)/)
    expect(presets).not.toContain('overview-accent')
  })

  it('cards do not lift on hover (no-lift contract)', () => {
    const css = read('src/styles/index.css')
    expect(css).not.toMatch(/data-slot='card'.*translateY\(-1px\)/s)
    expect(css).toContain('var(--shadow-card-hover)')
  })

  it('theme.css defines destructive-soft-fg in both themes (contrast fixes)', () => {
    const theme = read('src/styles/theme.css')
    expect(theme).toContain(
      '--color-destructive-soft-fg: var(--destructive-soft-fg)'
    )
    // Light ships a standalone darker ink (aliasing --destructive only
    // reached ~4.0:1 on the destructive/10 tint); dark keeps the lighter
    // readable value so soft-destructive text clears AA on dark surfaces.
    // Pinned by contrast-gate.test.ts; see a11y-checklist.md §4/§7.
    expect(theme).toMatch(/--destructive-soft-fg: oklch\(0\.5 0\.2 27\)/)
    expect(theme).toMatch(/--destructive-soft-fg: oklch\(0\.8 0\.15 22\)/)
  })
})
