// Theme contrast gate.
//
// Statically parses the OKLCH tokens from theme.css / theme-presets.css,
// resolves the cascade (base -> preset block -> semantic surface bridge),
// converts OKLCH -> OKLab -> linear sRGB -> gamma sRGB, and computes WCAG
// 2.x contrast ratios (same pipeline as the 2026-08-23 theme audit and
// scripts/dark-contrast-probe.mjs luminance math). Every tracked
// foreground/background pair must clear AA 4.5:1 across all 10 presets x
// {light, dark}, except a small documented exemption list of known
// residuals (a11y-checklist.md §7).
//
// Locks in the 2026-08-23 contrast fixes:
//   1. rose-garden dark --secondary (was 1.50:1, lowest ratio shipped)
//   2. default dark --destructive (solid destructive, was 2.77:1)
//   3. five preset dark --primary CTAs (were 3.59-3.69:1 with white text)
//   4. anthropic light clay --primary + sky --info (were 2.91 / 2.85:1)
//   5. light --destructive-soft-fg + dark --info-soft-fg soft badges
//   6. default light --sidebar-accent-foreground + the removed pure-black
//      lake-view dark override (were 3.95 / 1.75:1)

import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const read = (path: string) => readFileSync(join(WEB_ROOT, path), 'utf8')

const AA = 4.5

// ---------- color math (OKLCH -> sRGB -> WCAG) ----------

type Oklch = { l: number; c: number; h: number; a: number }
type Rgb = [number, number, number]

function oklchToLinearRgb({ l, c, h }: Oklch): Rgb {
  const hr = (h * Math.PI) / 180
  const a = c * Math.cos(hr)
  const b = c * Math.sin(hr)
  const l_ = l + 0.3963377774 * a + 0.2158037573 * b
  const m_ = l - 0.1055613458 * a - 0.0638541728 * b
  const s_ = l - 0.0894841775 * a - 1.291485548 * b
  const L = l_ ** 3
  const M = m_ ** 3
  const S = s_ ** 3
  return [
    4.0767416621 * L - 3.3077115913 * M + 0.2309699292 * S,
    -1.2684380046 * L + 2.6097574011 * M - 0.3413193965 * S,
    -0.0041960863 * L - 0.7034186147 * M + 1.707614701 * S,
  ]
}

function clamp01(v: number): number {
  return Math.min(1, Math.max(0, v))
}

function linearToGamma(v: number): number {
  const c = clamp01(v)
  return c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055
}

function oklchToSrgb(color: Oklch): Rgb {
  const [r, g, b] = oklchToLinearRgb(color)
  return [linearToGamma(r), linearToGamma(g), linearToGamma(b)]
}

function wcagLuminance([r, g, b]: Rgb): number {
  const f = (v: number) => {
    const c = clamp01(v)
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}

function contrastRatio(fg: Rgb, bg: Rgb): number {
  const l1 = wcagLuminance(fg)
  const l2 = wcagLuminance(bg)
  const hi = Math.max(l1, l2)
  const lo = Math.min(l1, l2)
  return (hi + 0.05) / (lo + 0.05)
}

/** Alpha-composite fg over bg in gamma sRGB (how browsers stack bg-x/NN). */
function composite(fg: Rgb, alpha: number, bg: Rgb): Rgb {
  return [
    alpha * fg[0] + (1 - alpha) * bg[0],
    alpha * fg[1] + (1 - alpha) * bg[1],
    alpha * fg[2] + (1 - alpha) * bg[2],
  ]
}

/** color-mix(in oklch, A p%, B): L/C linear; hue from the chromatic side. */
function mixOklch(a: Oklch, p: number, b: Oklch): Oklch {
  const l = p * a.l + (1 - p) * b.l
  const c = p * a.c + (1 - p) * b.c
  let h: number
  if (a.c <= 1e-9) h = b.h
  else if (b.c <= 1e-9) h = a.h
  else {
    let d = (b.h - a.h) % 360
    if (d > 180) d -= 360
    h = (a.h + p * d + 360) % 360
  }
  return { l, c, h, a: p * a.a + (1 - p) * b.a }
}

// ---------- CSS parsing ----------

const OKLCH_RE =
  /^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*(?:\/\s*([\d.]+)(%?))?\s*\)$/
const VAR_RE = /^var\(\s*(--[\w-]+)\s*\)$/
const MIX_RE =
  /^color-mix\(\s*in oklch,\s*var\(\s*(--[\w-]+)\s*\)\s*([\d.]+)%\s*,\s*var\(\s*(--[\w-]+)\s*\)\s*\)$/

type BlockKind =
  | 'root'
  | 'dark'
  | 'preset'
  | 'preset-dark'
  | 'bridge'
  | 'bridge-dark'
type TokenMap = Record<string, string>

function parseBlocks(
  css: string
): Array<{ selector: string; decls: TokenMap }> {
  const stripped = css.replaceAll(/\/\*[\s\S]*?\*\//g, '')
  const blocks: Array<{ selector: string; decls: TokenMap }> = []
  let i = 0
  while (i < stripped.length) {
    const brace = stripped.indexOf('{', i)
    if (brace === -1) break
    const selector = stripped.slice(i, brace).trim()
    let depth = 1
    let j = brace + 1
    while (j < stripped.length && depth > 0) {
      if (stripped[j] === '{') depth += 1
      else if (stripped[j] === '}') depth -= 1
      j += 1
    }
    const body = stripped.slice(brace + 1, j - 1)
    const decls: TokenMap = {}
    for (const m of body.matchAll(/(--[\w-]+)\s*:\s*([^;{}]+);/g)) {
      decls[m[1]] = m[2].split(/\s+/).join(' ')
    }
    if (Object.keys(decls).length > 0) blocks.push({ selector, decls })
    i = j
  }
  return blocks
}

function classify(selector: string): {
  kind: BlockKind | null
  preset: string | null
} {
  const s = selector.split(/\s+/).join(' ')
  const presetMatch = s.match(/\[data-theme-preset='([\w-]+)'\]/)
  if (presetMatch && !s.includes(':not(')) {
    return {
      kind: s.startsWith('.dark') ? 'preset-dark' : 'preset',
      preset: presetMatch[1],
    }
  }
  if (s.includes(":not([data-theme-preset='default'])")) {
    return {
      kind: s.startsWith('.dark') ? 'bridge-dark' : 'bridge',
      preset: null,
    }
  }
  if (s === ':root') return { kind: 'root', preset: null }
  if (s === '.dark') return { kind: 'dark', preset: null }
  return { kind: null, preset: null }
}

const PRESETS = [
  'default',
  'underground',
  'rose-garden',
  'lake-view',
  'sunset-glow',
  'forest-whisper',
  'ocean-breeze',
  'lavender-dream',
  'simple-large',
  'anthropic',
]

// Presets tinted by the semantic surface bridge in theme-presets.css.
const BRIDGE_PRESETS = new Set([
  'underground',
  'rose-garden',
  'lake-view',
  'sunset-glow',
  'forest-whisper',
  'ocean-breeze',
  'lavender-dream',
])

function loadTokenData() {
  const data: Record<BlockKind, Record<string, TokenMap>> = {
    root: { '': {} },
    dark: { '': {} },
    preset: {},
    'preset-dark': {},
    bridge: { '': {} },
    'bridge-dark': { '': {} },
  }
  for (const file of ['src/styles/theme.css', 'src/styles/theme-presets.css']) {
    for (const { selector, decls } of parseBlocks(read(file))) {
      const { kind, preset } = classify(selector)
      if (!kind) continue
      if (kind === 'preset' || kind === 'preset-dark') {
        data[kind][preset ?? ''] = { ...data[kind][preset ?? ''], ...decls }
      } else {
        data[kind][''] = { ...data[kind][''], ...decls }
      }
    }
  }
  return data
}

const DATA = loadTokenData()

/** Effective raw token map for preset x mode, honoring cascade specificity:
 * bridge (0,3,0) > preset block (0,2,0) > theme.css base (0,1,0). */
function effectiveTokens(preset: string, mode: 'light' | 'dark'): TokenMap {
  const tokens: TokenMap = {
    ...(mode === 'dark' ? DATA.dark[''] : DATA.root['']),
  }
  const bridgeKeys = new Set<string>()
  if (BRIDGE_PRESETS.has(preset)) {
    const bridge = DATA[mode === 'dark' ? 'bridge-dark' : 'bridge']['']
    Object.assign(tokens, bridge)
    for (const key of Object.keys(bridge)) bridgeKeys.add(key)
  }
  const block = DATA[mode === 'dark' ? 'preset-dark' : 'preset'][preset] ?? {}
  for (const [key, value] of Object.entries(block)) {
    if (!bridgeKeys.has(key)) tokens[key] = value
  }
  return tokens
}

function resolveValue(
  raw: string | undefined,
  tokens: TokenMap,
  depth = 0
): Oklch | null {
  if (!raw || depth > 12) return null
  const mix = raw.match(MIX_RE)
  if (mix) {
    const a = resolveValue(tokens[mix[1]], tokens, depth + 1)
    const b = resolveValue(tokens[mix[3]], tokens, depth + 1)
    if (a && b) return mixOklch(a, Number(mix[2]) / 100, b)
    return null
  }
  const varRef = raw.match(VAR_RE)
  if (varRef) return resolveValue(tokens[varRef[1]], tokens, depth + 1)
  const oklch = raw.match(OKLCH_RE)
  if (oklch) {
    const alpha = oklch[4] ? Number(oklch[4]) / (oklch[5] ? 100 : 1) : 1
    return {
      l: Number(oklch[1]),
      c: Number(oklch[2]),
      h: Number(oklch[3]),
      a: alpha,
    }
  }
  return null
}

function tokenRgb(
  preset: string,
  mode: 'light' | 'dark',
  token: string
): Rgb | null {
  const tokens = effectiveTokens(preset, mode)
  const color = resolveValue(tokens[`--${token}`], tokens)
  if (!color) return null
  return oklchToSrgb(color)
}

function pairRatio(
  preset: string,
  mode: 'light' | 'dark',
  fg: string,
  bg: string
): number | null {
  const f = tokenRgb(preset, mode, fg)
  const b = tokenRgb(preset, mode, bg)
  if (!f || !b) return null
  return contrastRatio(f, b)
}

/** Soft badges render text-<tone>-soft-fg on bg-<tone>/10 (destructive uses
 * /20 in dark per badge.tsx/button.tsx) composited over the card surface. */
function softRatio(
  preset: string,
  mode: 'light' | 'dark',
  tone: string
): number | null {
  const tokens = effectiveTokens(preset, mode)
  const fg = resolveValue(tokens[`--${tone}-soft-fg`], tokens)
  const base = resolveValue(tokens[`--${tone}`], tokens)
  const card = resolveValue(tokens['--card'], tokens)
  if (!fg || !base || !card) return null
  const alpha = tone === 'destructive' && mode === 'dark' ? 0.2 : 0.1
  const fill = composite(oklchToSrgb(base), alpha, oklchToSrgb(card))
  return contrastRatio(oklchToSrgb(fg), fill)
}

// ---------- audit surface ----------

const SOLID_PAIRS: Array<[string, string]> = [
  ['primary-foreground', 'primary'],
  ['secondary-foreground', 'secondary'],
  ['destructive-foreground', 'destructive'],
  ['info-foreground', 'info'],
  ['success-foreground', 'success'],
  ['warning-foreground', 'warning'],
  ['foreground', 'background'],
  ['foreground', 'card'],
  ['muted-foreground', 'card'],
  ['accent-foreground', 'accent'],
  ['sidebar-accent-foreground', 'sidebar-accent'],
]
const SOFT_TONES = ['destructive', 'info', 'success', 'warning']

/** Known residuals outside the 2026-08-23 fix scope (a11y-checklist §7).
 * Key: `${preset}|${mode}|${label}`. */
const EXEMPTIONS: Record<string, string> = {}

describe('theme contrast gate (WCAG AA 4.5:1)', () => {
  it('every tracked pair passes AA across 10 presets x both modes (or is a documented exemption)', () => {
    const failures: string[] = []
    for (const preset of PRESETS) {
      for (const mode of ['light', 'dark'] as const) {
        for (const [fg, bg] of SOLID_PAIRS) {
          const label = `${fg} on ${bg}`
          const ratio = pairRatio(preset, mode, fg, bg)
          if (ratio === null) {
            failures.push(`${preset} ${mode}: ${label} could not be resolved`)
            continue
          }
          const key = `${preset}|${mode}|${label}`
          if (ratio < AA && !(key in EXEMPTIONS)) {
            failures.push(
              `${preset} ${mode}: ${label} = ${ratio.toFixed(2)} (< ${AA})`
            )
          }
        }
        for (const tone of SOFT_TONES) {
          const alpha = tone === 'destructive' && mode === 'dark' ? 20 : 10
          const label = `${tone}-soft-fg on ${tone}/${alpha}`
          const ratio = softRatio(preset, mode, tone)
          if (ratio === null) {
            failures.push(`${preset} ${mode}: ${label} could not be resolved`)
            continue
          }
          const key = `${preset}|${mode}|${label}`
          if (ratio < AA && !(key in EXEMPTIONS)) {
            failures.push(
              `${preset} ${mode}: ${label} = ${ratio.toFixed(2)} (< ${AA})`
            )
          }
        }
      }
    }
    expect(failures).toEqual([])
  })

  it('exemptions still reference pairs that exist and stay below 6:1 (no silent scope creep)', () => {
    for (const key of Object.keys(EXEMPTIONS)) {
      const [preset, mode, label] = key.split('|') as [
        string,
        'light' | 'dark',
        string,
      ]
      const match = label.match(/^(.+?) on (.+)$/)
      expect(match, `exemption key malformed: ${key}`).not.toBeNull()
      if (!match) continue
      const [, fg, bg] = match
      const ratio = bg.includes('/')
        ? softRatio(preset, mode, bg.split('/')[0])
        : pairRatio(preset, mode, fg, bg)
      expect(ratio, `exemption no longer resolves: ${key}`).not.toBeNull()
      if (ratio !== null) expect(ratio).toBeLessThan(6)
    }
  })

  it('the six 2026-08-23 fixes keep their committed token values', () => {
    const val = (preset: string, mode: 'light' | 'dark', token: string) => {
      const tokens = effectiveTokens(preset, mode)
      return resolveValue(tokens[`--${token}`], tokens)
    }
    const close = (actual: Oklch | null, l: number, c: number, h: number) => {
      expect(actual, 'token resolves').not.toBeNull()
      if (!actual) return
      expect(actual.l).toBeCloseTo(l, 4)
      expect(actual.c).toBeCloseTo(c, 4)
      expect(actual.h).toBeCloseTo(h, 2)
    }
    // 1. rose-garden dark secondary: dark rose surface, was light pink 1.50:1
    close(val('rose-garden', 'dark', 'secondary'), 0.4, 0.08, 8)
    // 2. default dark destructive darkened for white foreground
    close(val('default', 'dark', 'destructive'), 0.575, 0.19, 25)
    // 3. five preset dark primaries darkened for white CTA text
    close(val('forest-whisper', 'dark', 'primary'), 0.529, 0.12, 180.39)
    close(val('ocean-breeze', 'dark', 'primary'), 0.563, 0.188, 259.81)
    close(val('lavender-dream', 'dark', 'primary'), 0.5759, 0.1699, 307.95)
    close(val('rose-garden', 'dark', 'primary'), 0.5876, 0.2348, 10.36)
    close(val('sunset-glow', 'dark', 'primary'), 0.5787, 0.1822, 23.51)
    // 4. anthropic light clay + sky darkened for AA
    close(val('anthropic', 'light', 'primary'), 0.57, 0.15, 38)
    close(val('anthropic', 'light', 'info'), 0.55, 0.075, 248)
    // 5. independent soft-badge foregrounds
    close(val('default', 'light', 'destructive-soft-fg'), 0.5, 0.2, 27)
    close(val('default', 'dark', 'info-soft-fg'), 0.72, 0.12, 245)
    // 6. sidebar active item: darkened default light foreground; the
    //    pure-black lake-view dark override was deleted (falls back to
    //    theme.css dark).
    close(
      val('default', 'light', 'sidebar-accent-foreground'),
      0.505,
      0.18,
      256
    )
    const lakeViewDark = DATA['preset-dark']['lake-view'] ?? {}
    expect(lakeViewDark['--sidebar-accent-foreground']).toBeUndefined()
  })
})
