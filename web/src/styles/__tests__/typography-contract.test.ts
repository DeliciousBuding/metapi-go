import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const SRC = join(WEB_ROOT, 'src')
const read = (path: string) => readFileSync(join(WEB_ROOT, path), 'utf8')

function walkSources(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (entry === 'node_modules' || entry === '__tests__') continue
      out.push(...walkSources(full))
    } else if (/\.(ts|tsx)$/.test(entry) && !/\.test\./.test(entry)) {
      out.push(full)
    }
  }
  return out
}

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

  it('keeps the two page-title roles readable as density changes', () => {
    const styles = read('src/styles/index.css')
    for (const [role, size] of [
      ['page-title', 'text-xl'],
      ['page-title-overview', 'text-2xl'],
    ]) {
      const body = styles.match(
        new RegExp(`@utility ${role}\\s*\\{([^}]+)\\}`)
      )?.[1]
      expect(body).toBeDefined()
      for (const token of [
        size,
        'font-semibold',
        'leading-snug',
        'text-balance',
      ]) {
        expect(body).toContain(token)
      }
    }
  })

  it('lets table cell components own their secondary typography', () => {
    for (const file of [
      'src/components/ui/table.tsx',
      'src/components/data-table/core/data-table-view.tsx',
    ]) {
      const source = read(file)
      expect(source).not.toMatch(/\[&_t[dh](?:_\*)?\]:text-/)
    }
  })

  it('keeps product code off arbitrary font-size classes', () => {
    // `text-[<n>px]` / `text-[<n>rem]` freeze a size at one density, which
    // inverts the scale hierarchy under `data-theme-scale` (a hard-coded
    // 11px caption ends up *smaller* than the scaled `text-xs` it sits
    // under at `lg`). Caption sizes ride the density axis through the
    // registered `--text-2xs` / `--text-3xs` tokens (theme.css). Exemptions
    // require a per-file reason — none expected.
    const exemptions: { file: string; reason: string }[] = []
    const exemptedFiles = new Set(exemptions.map((e) => e.file))

    const offenders = walkSources(SRC)
      .filter((file) => !exemptedFiles.has(file.slice(SRC.length + 1)))
      .flatMap((file) => {
        const matches = readFileSync(file, 'utf8').match(
          /text-\[\d+(?:\.\d+)?(?:px|rem)\]/g
        )
        return matches
          ? matches.map((m) => `${file.slice(SRC.length + 1)}: ${m}`)
          : []
      })
    expect(offenders).toEqual([])
  })

  it('registers the sub-xs caption tokens and scales them with density', () => {
    // Without the @theme registration the `text-2xs` / `text-3xs` utilities
    // stop generating and every caption silently falls back to the inherited
    // size; without the preset overrides they freeze at the md density.
    const theme = read('src/styles/theme.css')
    expect(theme).toContain('--text-2xs: 0.6875rem')
    expect(theme).toContain('--text-3xs: 0.625rem')

    const presets = read('src/styles/theme-presets.css')
    for (const scale of ['sm', 'lg', 'xl']) {
      const block = presets.match(
        new RegExp(`\\[data-theme-scale='${scale}'\\]\\s*\\{([^}]+)\\}`)
      )?.[1]
      expect(block, `density ${scale}`).toBeDefined()
      expect(block).toContain('--text-2xs:')
      expect(block).toContain('--text-3xs:')
    }
  })

  it('keeps density scaling out of color presets', () => {
    const presets = read('src/styles/theme-presets.css')
    const graphite = presets.match(
      /\[data-theme-preset='graphite'\]\s*\{([\s\S]*?)\n\}/
    )?.[1]

    expect(graphite).toBeDefined()
    expect(graphite).not.toMatch(/--text-|--spacing/)
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
