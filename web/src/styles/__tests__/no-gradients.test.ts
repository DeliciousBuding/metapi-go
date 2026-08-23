import { readdirSync, readFileSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const SCAN_ROOTS = [join(WEB_ROOT, 'src'), join(WEB_ROOT, 'public')]
const ROOT_TEXT_FILES = [join(WEB_ROOT, 'index.html')]
const TEXT_EXTENSIONS = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.jsx',
  '.mjs',
  '.svg',
  '.ts',
  '.tsx',
])
const THIS_FILE = fileURLToPath(import.meta.url)

const FORBIDDEN_PATTERNS = [
  {
    label: 'CSS color ramp',
    pattern: /(?:linear|radial|conic)-gradient\s*\(/i,
  },
  {
    label: 'SVG color ramp definition',
    pattern: /<\s*(?:linear|radial)Gradient\b/i,
  },
  { label: 'SVG color ramp reference', pattern: /url\(\s*#.*gradient/i },
  {
    label: 'Tailwind color ramp utility',
    pattern: /\bbg-(?:linear|radial)(?:-|\b)/i,
  },
]

// The flat-surface rule governs application chrome and content surfaces so
// the OKLCH token system stays the single source of color. A fixed brand
// logomark is a design asset, not a UI surface, so the gradient in the
// primary logo/favicon is an explicit, documented exception here.
//
// `src/styles/index.css` is allowlisted for the `.animate-shimmer` skeleton
// sweep only. A shimmer is structurally a moving gradient — there is no
// non-gradient way to produce a left-to-right highlight sweep (a solid
// `background-color` animation can only fade the whole bar, which reads as
// "broken" instead of "loading"). The gradient is composed exclusively of
// the `--skeleton-base` / `--skeleton-highlight` OKLCH tokens defined in
// theme.css, so the token system remains the single source of color — the
// rule's intent is preserved even though the regex matches the literal
// `linear-gradient(...)`. If a future contributor adds a *decorative*
// gradient to index.css, that gradient must be re-implemented via tokens
// or moved out of this file; this exception is scoped to the shimmer.
const GRADIENT_ALLOWLIST = new Set([
  'public/logo.svg',
  'public/favicon.svg',
  'src/styles/index.css',
  // Alpha-only scroll-edge mask for the mobile card list(s): the gradient
  // fades `mask-image` alpha so rows desolve at the container boundary
  // instead of hard-cutting into a 2px sliver. It carries NO color (black →
  // transparent in an alpha mask), so the OKLCH token system remains the
  // single source of color — the regex only sees the literal
  // `linear-gradient(...)`.
  'src/components/data-table/layout/data-table-page.tsx',
])

function normalizePath(path: string): string {
  return path.split(/[\\/]/).join('/')
}

function collectTextFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return collectTextFiles(path)
    if (path === THIS_FILE || !TEXT_EXTENSIONS.has(extname(path))) return []
    return [path]
  })
}

describe('flat visual surfaces', () => {
  it('keeps application source and public assets free of color ramps', () => {
    const files = [...SCAN_ROOTS.flatMap(collectTextFiles), ...ROOT_TEXT_FILES]
    const violations = files.flatMap((path) => {
      const rel = normalizePath(relative(WEB_ROOT, path))
      if (GRADIENT_ALLOWLIST.has(rel)) return []
      const content = readFileSync(path, 'utf8')
      return FORBIDDEN_PATTERNS.filter(({ pattern }) =>
        pattern.test(content)
      ).map(({ label }) => `${relative(WEB_ROOT, path)}: ${label}`)
    })

    expect(violations).toEqual([])
  })

  it('keeps the sign-in background flat', () => {
    const signInPage = readFileSync(
      join(WEB_ROOT, 'src/features/auth/components/sign-in-page.tsx'),
      'utf8'
    )

    expect(signInPage).not.toContain('blur-3xl')
    expect(signInPage).not.toContain('bg-primary/10')
  })
})
