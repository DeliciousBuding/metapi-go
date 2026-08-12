import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const read = (path: string) => readFileSync(join(WEB_ROOT, path), 'utf8')

describe('typography design contract', () => {
  it('uses the bundled variable sans face and a project-owned mono stack', () => {
    const theme = read('src/styles/theme.css')

    expect(theme).toContain("--font-sans:")
    expect(theme).toContain("'Public Sans Variable', 'Public Sans'")
    expect(theme).toContain("'Noto Sans SC'")
    expect(theme).toMatch(/--font-mono:\s*'Cascadia Mono'/)
    expect(theme).not.toContain('--font-inter:')
    expect(theme).not.toContain('--font-manrope:')
  })

  it('keeps component typography on semantic font tokens', () => {
    expect(read('src/styles/index.css')).not.toContain(
      '@apply overflow-x-hidden font-sans'
    )

    const kbd = read('src/components/ui/kbd.tsx')
    expect(kbd).toContain('font-mono')
    expect(kbd).not.toContain('font-sans')
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
    const html = read('index.html')

    expect(html).toContain("readCookie('theme_preset')")
    expect(html).toContain("readCookie('theme_font')")
    expect(html).toContain("'var(--background, var(--bootstrap-background))'")
  })
})
