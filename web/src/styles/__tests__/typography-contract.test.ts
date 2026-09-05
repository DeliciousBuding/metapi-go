import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const read = (path: string) => readFileSync(join(WEB_ROOT, path), 'utf8')

describe('typography design contract', () => {
  it('uses the bundled variable sans face and a project-owned mono stack', () => {
    const theme = read('src/styles/theme.css')

    expect(theme).toContain('--font-sans:')
    expect(theme).toContain("'Public Sans Variable', 'Public Sans'")
    expect(theme).toContain("'Noto Sans SC'")
    expect(theme).toMatch(/--font-mono:\s*'Cascadia Mono'/)
    expect(theme).not.toContain('--font-inter:')
    expect(theme).not.toContain('--font-manrope:')
  })

  it('ships a bundled CJK face ahead of the platform CJK fallbacks', () => {
    const theme = read('src/styles/theme.css')
    const sans = theme.slice(theme.indexOf('--font-sans:'))
    // Per-glyph fallback walks the stack in order: the bundled variable face
    // must sit in front of the platform CJK fonts, which are insurance for a
    // failed fetch only. Before the face was bundled the stack jumped from
    // Public Sans straight to them, so every Chinese string rendered in
    // whatever the OS shipped.
    const bundled = sans.indexOf("'Noto Sans SC Variable'")
    expect(bundled).toBeGreaterThan(-1)
    expect(bundled).toBeLessThan(sans.indexOf("'Microsoft YaHei'"))
    // Naming a family in a stack ships nothing — the @font-face lives in the
    // fontsource package, so the import is the other half of the contract.
    expect(read('src/styles/index.css')).toContain(
      "@import '@fontsource-variable/noto-sans-sc'"
    )
  })

  it('neutralises Latin display tracking for a Chinese UI', () => {
    // Negative tracking is a Latin display convention; an ideograph's side
    // bearing is its only spacing, so `tracking-tight` made Chinese headings
    // touch. The token override is keyed on `<html lang>`, which i18n keeps
    // in sync with the active language.
    const theme = read('src/styles/theme.css')
    const zh = theme.slice(theme.indexOf(":root[lang|='zh']"))
    expect(zh).toContain('--tracking-tight: 0em')
  })

  it('keeps component typography on semantic font tokens', () => {
    expect(read('src/styles/index.css')).not.toContain(
      '@apply overflow-x-hidden font-sans'
    )

    const secretField = read('src/components/ui/secret-field.tsx')
    expect(secretField).toContain('font-mono')
    expect(secretField).not.toContain('font-sans')
  })

  it('keeps density scaling out of color presets', () => {
    const presets = read('src/styles/theme-presets.css')
    const simpleLarge = presets.match(
      /\[data-theme-preset='simple-large'\]\s*\{([\s\S]*?)\n\}/
    )?.[1]

    expect(simpleLarge).toBeDefined()
    expect(simpleLarge).not.toMatch(/--text-|--spacing/)
  })

  it('hydrates persisted visual axes before the app mounts', () => {
    const script = read('public/theme-init.js')

    expect(script).toContain("readCookie('theme_preset')")
    expect(script).toContain("readCookie('theme_font')")
  })

  it('paints the boot background through the design token with a fallback', () => {
    const script = read('public/bootstrap.js')

    expect(script).toContain("'var(--background, var(--bootstrap-background))'")
  })
})
